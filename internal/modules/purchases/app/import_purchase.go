package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/GARFEX33/garfex-costos-unitarios/cfdicore"
	"github.com/GARFEX33/garfex-costos-unitarios/internal/modules/purchases/domain"
	supplierdomain "github.com/GARFEX33/garfex-costos-unitarios/internal/modules/suppliers/domain"
	"github.com/shopspring/decimal"
)

// ImportOptions carries the caller-supplied context a CFDI itself does not
// declare: which branch of the resolved supplier the purchase belongs to,
// and the original filename kept only for traceability.
type ImportOptions struct {
	// BranchID is optional; a document with no applicable branch leaves it
	// zero.
	BranchID int64
	Filename string
}

// ImportCFDI registers one CFDI 4.0 purchase document from its raw XML
// bytes. It is idempotent on the document's fiscal UUID: re-importing the
// same document returns the existing purchase with
// ImportResult.AlreadyExisted set, importing the same UUID with different
// relevant content returns domain.ErrPurchaseConflict, and the supplier
// issuing the document is looked up by tax identifier, minimally registered
// on first sight when unknown. The whole write is atomic: it is delegated
// to Repository.Import, which persists the purchase, every line, and the
// resolved supplier-product relations, or persists nothing.
func (s *Service) ImportCFDI(ctx context.Context, xml []byte, opts ImportOptions) (domain.ImportResult, error) {
	if len(xml) == 0 {
		return domain.ImportResult{}, domain.NewValidationError("xml_content", "is required")
	}

	invoice, err := cfdicore.Parse(xml)
	if err != nil {
		return domain.ImportResult{}, domain.NewValidationError("xml_content", "not a usable CFDI 4.0 document: "+err.Error())
	}
	if invoice.Stamp == nil || strings.TrimSpace(invoice.Stamp.UUID) == "" {
		return domain.ImportResult{}, domain.NewValidationError("cfdi_uuid", "document carries no fiscal stamp (TimbreFiscalDigital)")
	}

	supplierID, err := s.resolveSupplier(ctx, invoice.Issuer.TaxID, invoice.Issuer.Name)
	if err != nil {
		return domain.ImportResult{}, wrap("resolve purchase supplier", err)
	}

	var branchID *int64
	if opts.BranchID > 0 {
		branch, err := s.suppliers.GetBranch(ctx, supplierID, opts.BranchID)
		if err != nil {
			return domain.ImportResult{}, wrap("resolve purchase branch", err)
		}
		branchID = &branch.ID
	}

	draft, err := buildPurchaseDraft(invoice, xml, supplierID, branchID, opts.Filename)
	if err != nil {
		return domain.ImportResult{}, err
	}

	result, err := s.repo.Import(ctx, draft)
	return result, wrap("import purchase", err)
}

func (s *Service) resolveSupplier(ctx context.Context, taxID, name string) (int64, error) {
	taxID = strings.ToUpper(strings.TrimSpace(taxID))
	if taxID == "" {
		return 0, domain.NewValidationError("issuer_tax_id", "is required")
	}

	supplier, err := s.suppliers.GetSupplierByTaxIdentifier(ctx, taxID)
	if err == nil {
		return supplier.ID, nil
	}
	if !errors.Is(err, supplierdomain.ErrNotFound) {
		return 0, err
	}

	created, err := s.suppliers.CreateSupplier(ctx, supplierdomain.SupplierDetails{
		LegalName:     name,
		TaxIdentifier: taxID,
	})
	if err == nil {
		return created.ID, nil
	}
	if !errors.Is(err, supplierdomain.ErrConflict) {
		return 0, err
	}

	// A concurrent import registered the same supplier between the lookup
	// above and this create attempt; the tax identifier is already claimed,
	// so reuse it instead of failing.
	supplier, err = s.suppliers.GetSupplierByTaxIdentifier(ctx, taxID)
	if err != nil {
		return 0, err
	}
	return supplier.ID, nil
}

func buildPurchaseDraft(invoice cfdicore.Invoice, xml []byte, supplierID int64, branchID *int64, filename string) (domain.PurchaseDraft, error) {
	subtotal, err := parseDecimal("subtotal", invoice.Subtotal)
	if err != nil {
		return domain.PurchaseDraft{}, err
	}
	discount, err := parseDecimal("discount", invoice.Discount)
	if err != nil {
		return domain.PurchaseDraft{}, err
	}
	taxTransferred, err := parseDecimal("tax_transferred", invoice.Taxes.TotalTransferred)
	if err != nil {
		return domain.PurchaseDraft{}, err
	}
	taxWithheld, err := parseDecimal("tax_withheld", invoice.Taxes.TotalWithheld)
	if err != nil {
		return domain.PurchaseDraft{}, err
	}
	total, err := parseDecimal("total", invoice.Total)
	if err != nil {
		return domain.PurchaseDraft{}, err
	}
	exchangeRate, err := parseOptionalDecimal("exchange_rate", invoice.ExchangeRate)
	if err != nil {
		return domain.PurchaseDraft{}, err
	}

	lines := make([]domain.PurchaseLineDraft, 0, len(invoice.Concepts))
	for i, concept := range invoice.Concepts {
		line, err := buildLineDraft(i+1, concept)
		if err != nil {
			return domain.PurchaseDraft{}, err
		}
		lines = append(lines, line)
	}

	sum := sha256.Sum256(xml)
	return domain.NewPurchaseDraft(domain.PurchaseDraft{
		Supplier:       domain.PurchaseParty{SupplierID: supplierID, BranchID: branchID},
		CFDIUUID:       invoice.Stamp.UUID,
		Series:         invoice.Series,
		Folio:          invoice.Folio,
		IssuedAt:       invoice.IssuedAt,
		Currency:       invoice.Currency,
		ExchangeRate:   exchangeRate,
		Subtotal:       subtotal,
		Discount:       discount,
		TaxTransferred: taxTransferred,
		TaxWithheld:    taxWithheld,
		Total:          total,
		IssuerTaxID:    invoice.Issuer.TaxID,
		IssuerName:     invoice.Issuer.Name,
		XML: domain.XMLDocument{
			Content:  xml,
			Hash:     hex.EncodeToString(sum[:]),
			Filename: filename,
		},
		Lines: lines,
	})
}

func buildLineDraft(lineNumber int, concept cfdicore.Concept) (domain.PurchaseLineDraft, error) {
	quantity, err := parseDecimal("lines.quantity", concept.Quantity)
	if err != nil {
		return domain.PurchaseLineDraft{}, err
	}
	unitPrice, err := parseDecimal("lines.unit_price", concept.UnitPrice)
	if err != nil {
		return domain.PurchaseLineDraft{}, err
	}
	amount, err := parseDecimal("lines.amount", concept.Amount)
	if err != nil {
		return domain.PurchaseLineDraft{}, err
	}
	discount, err := parseDecimal("lines.discount", concept.Discount)
	if err != nil {
		return domain.PurchaseLineDraft{}, err
	}
	taxTransferred, err := sumTaxes("lines.tax_transferred", concept.Taxes.Transferred)
	if err != nil {
		return domain.PurchaseLineDraft{}, err
	}
	taxWithheld, err := sumTaxes("lines.tax_withheld", concept.Taxes.Withheld)
	if err != nil {
		return domain.PurchaseLineDraft{}, err
	}

	return domain.PurchaseLineDraft{
		LineNumber:     lineNumber,
		Description:    concept.Description,
		SupplierSKU:    concept.ItemNumber,
		SATProductCode: concept.ProductServiceCode,
		Quantity:       quantity,
		UnitCode:       concept.UnitCode,
		Unit:           concept.Unit,
		UnitPrice:      unitPrice,
		Amount:         amount,
		Discount:       discount,
		TaxTransferred: taxTransferred,
		TaxWithheld:    taxWithheld,
		TaxObject:      concept.TaxObject,
	}, nil
}

func sumTaxes(field string, taxes []cfdicore.Tax) (decimal.Decimal, error) {
	total := decimal.Zero
	for _, tax := range taxes {
		amount, err := parseDecimal(field, tax.Amount)
		if err != nil {
			return decimal.Decimal{}, err
		}
		total = total.Add(amount)
	}
	return total, nil
}

func parseDecimal(field, raw string) (decimal.Decimal, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return decimal.Zero, nil
	}
	value, err := decimal.NewFromString(raw)
	if err != nil {
		return decimal.Decimal{}, domain.NewValidationError(field, "is not a valid decimal amount")
	}
	return value, nil
}

func parseOptionalDecimal(field, raw string) (*decimal.Decimal, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	value, err := decimal.NewFromString(raw)
	if err != nil {
		return nil, domain.NewValidationError(field, "is not a valid decimal amount")
	}
	return &value, nil
}
