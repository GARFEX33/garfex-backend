package domain

import (
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Purchase is one fiscal purchase document (CFDI) registered in the system.
// Every field mirrors data taken from the original XML; none of it is
// recomputed or normalized here. It is never modified after import except
// for the module-owned relation fields carried by its lines.
type Purchase struct {
	ID       int64
	Supplier PurchaseParty

	// CFDIUUID is the fiscal identity of the document (TimbreFiscalDigital
	// UUID), trimmed and upper-cased. It is the sole identity used to detect
	// duplicate imports of the same document.
	CFDIUUID string
	Series   string
	Folio    string
	// IssuedAt is the CFDI's own issuance date (Comprobante Fecha), the date
	// every price-history and evolution query anchors on. It is never the
	// import timestamp.
	IssuedAt       time.Time
	Currency       string
	ExchangeRate   *decimal.Decimal
	Subtotal       decimal.Decimal
	Discount       decimal.Decimal
	TaxTransferred decimal.Decimal
	TaxWithheld    decimal.Decimal
	Total          decimal.Decimal

	IssuerTaxID string
	IssuerName  string

	XML XMLDocument

	ImportedAt time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// PurchaseParty identifies the resolved supplier and, when applicable, the
// branch the purchase is attributed to.
type PurchaseParty struct {
	SupplierID int64
	BranchID   *int64
}

// XMLDocument preserves the original purchase document integrally.
type XMLDocument struct {
	Content []byte
	// Hash is the lower-case hex SHA-256 of Content, used to detect
	// byte-identical files independently of the fiscal UUID.
	Hash string
	// Filename is informational traceability only; it is never part of any
	// document's identity.
	Filename string
}

// PurchaseDraft carries everything required to import one purchase document.
// It is built by the application layer from a parsed CFDI and is validated,
// never partially, by NewPurchaseDraft.
type PurchaseDraft struct {
	Supplier PurchaseParty

	CFDIUUID       string
	Series         string
	Folio          string
	IssuedAt       time.Time
	Currency       string
	ExchangeRate   *decimal.Decimal
	Subtotal       decimal.Decimal
	Discount       decimal.Decimal
	TaxTransferred decimal.Decimal
	TaxWithheld    decimal.Decimal
	Total          decimal.Decimal

	IssuerTaxID string
	IssuerName  string

	XML XMLDocument

	Lines []PurchaseLineDraft
}

// NewPurchaseDraft validates draft and returns its canonical form. It
// returns a ValidationError naming the first offending field.
func NewPurchaseDraft(draft PurchaseDraft) (PurchaseDraft, error) {
	draft.CFDIUUID = strings.ToUpper(strings.TrimSpace(draft.CFDIUUID))
	draft.Series = strings.TrimSpace(draft.Series)
	draft.Folio = strings.TrimSpace(draft.Folio)
	draft.Currency = strings.ToUpper(strings.TrimSpace(draft.Currency))
	draft.IssuerTaxID = strings.ToUpper(strings.TrimSpace(draft.IssuerTaxID))
	draft.IssuerName = strings.TrimSpace(draft.IssuerName)
	draft.XML.Hash = strings.ToLower(strings.TrimSpace(draft.XML.Hash))
	draft.XML.Filename = strings.TrimSpace(draft.XML.Filename)

	if draft.Supplier.SupplierID <= 0 {
		return PurchaseDraft{}, NewValidationError("supplier_id", "must be positive")
	}
	if draft.Supplier.BranchID != nil && *draft.Supplier.BranchID <= 0 {
		return PurchaseDraft{}, NewValidationError("branch_id", "must be positive when present")
	}
	if draft.CFDIUUID == "" {
		return PurchaseDraft{}, NewValidationError("cfdi_uuid", "is required")
	}
	if draft.IssuedAt.IsZero() {
		return PurchaseDraft{}, NewValidationError("issued_at", "is required")
	}
	if draft.Currency == "" {
		return PurchaseDraft{}, NewValidationError("currency", "is required")
	}
	if draft.IssuerTaxID == "" {
		return PurchaseDraft{}, NewValidationError("issuer_tax_id", "is required")
	}
	if len(draft.XML.Content) == 0 {
		return PurchaseDraft{}, NewValidationError("xml_content", "is required")
	}
	if len(draft.XML.Hash) != 64 {
		return PurchaseDraft{}, NewValidationError("xml_hash", "must be a 64-character hex sha-256 digest")
	}
	if draft.Subtotal.IsNegative() {
		return PurchaseDraft{}, NewValidationError("subtotal", "must not be negative")
	}
	if draft.Total.IsNegative() {
		return PurchaseDraft{}, NewValidationError("total", "must not be negative")
	}
	if len(draft.Lines) == 0 {
		return PurchaseDraft{}, NewValidationError("lines", "at least one purchase line is required")
	}
	for i, line := range draft.Lines {
		canonical, err := newPurchaseLineDraft(line)
		if err != nil {
			return PurchaseDraft{}, err
		}
		draft.Lines[i] = canonical
	}
	return draft, nil
}

// SameRelevantContent reports whether draft describes the same fiscal
// document as an already-registered Purchase: same totals, currency, dates,
// and issuer. It deliberately ignores XML byte layout and import metadata.
// Callers that also hold the existing purchase's lines should additionally
// compare them with SameLines for a complete content check.
func (p Purchase) SameRelevantContent(draft PurchaseDraft) bool {
	return p.Series == draft.Series &&
		p.Folio == draft.Folio &&
		p.Currency == draft.Currency &&
		p.IssuerTaxID == draft.IssuerTaxID &&
		p.IssuedAt.Equal(draft.IssuedAt) &&
		p.Subtotal.Equal(draft.Subtotal) &&
		p.Discount.Equal(draft.Discount) &&
		p.TaxTransferred.Equal(draft.TaxTransferred) &&
		p.TaxWithheld.Equal(draft.TaxWithheld) &&
		p.Total.Equal(draft.Total)
}

// SameLines reports whether existing describes the same line items as
// draftLines: same count, and for each position the same description, SKU,
// quantity, unit price, and amount.
func SameLines(existing []PurchaseLine, draftLines []PurchaseLineDraft) bool {
	if len(existing) != len(draftLines) {
		return false
	}
	for i, line := range existing {
		draft := draftLines[i]
		if line.LineNumber != draft.LineNumber ||
			line.Description != draft.Description ||
			line.SupplierSKU != draft.SupplierSKU ||
			!line.Quantity.Equal(draft.Quantity) ||
			!line.UnitPrice.Equal(draft.UnitPrice) ||
			!line.Amount.Equal(draft.Amount) {
			return false
		}
	}
	return true
}
