package httpapi

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/GARFEX33/garfex-costos-unitarios/purchasecore"
)

const (
	defaultPurchaseLimit = 50
	maximumPurchaseLimit = 50
)

// PurchaseReader is the narrow Core capability required by the purchase,
// purchase-line, supplier-product, and resource price-history read routes.
type PurchaseReader interface {
	GetPurchase(context.Context, int64) (purchasecore.Purchase, error)
	GetPurchaseByUUID(context.Context, string) (purchasecore.Purchase, error)
	ListPurchaseLines(context.Context, int64) ([]purchasecore.PurchaseLine, error)
	ListPurchaseLinesWorkbench(context.Context, purchasecore.PurchaseLineQuery) (purchasecore.PurchaseLinePage, error)
	ListPurchasesBySupplier(context.Context, int64, purchasecore.ListCriteria) (purchasecore.PurchasePage, error)
	GetSupplierProduct(context.Context, int64) (purchasecore.SupplierProduct, error)
	FindSupplierProduct(context.Context, int64, string) (purchasecore.SupplierProduct, error)
	ListSupplierProducts(context.Context, int64, purchasecore.ListCriteria) (purchasecore.SupplierProductPage, error)
	ListPurchaseLinesByResource(context.Context, int64, purchasecore.ListCriteria) (purchasecore.PurchaseLineHistoryPage, error)
}

// PurchaseWriter is the narrow Core capability required by purchase import,
// explicit SupplierProduct mapping transitions, and line resolution.
type PurchaseWriter interface {
	ImportPurchase(context.Context, purchasecore.ImportRequest) (purchasecore.ImportResult, error)
	ConfirmMapping(context.Context, purchasecore.ConfirmMappingRequest) (purchasecore.SupplierProduct, error)
	CorrectMapping(context.Context, purchasecore.CorrectMappingRequest) (purchasecore.SupplierProduct, error)
	ExceptionalUnlink(context.Context, purchasecore.ExceptionalUnlinkRequest) (purchasecore.SupplierProduct, error)
	ReportIdentityConflict(context.Context, purchasecore.ReportIdentityConflictRequest) (purchasecore.SupplierProduct, error)
	ResolveIdentityConflict(context.Context, purchasecore.ResolveIdentityConflictRequest) (purchasecore.SupplierProduct, error)
	ResolvePurchaseLine(context.Context, purchasecore.ResolvePurchaseLineRequest) (purchasecore.ResolvePurchaseLineResult, error)
	SetResolutionOverride(context.Context, purchasecore.SetResolutionOverrideRequest) (purchasecore.PurchaseLine, error)
}

type purchaseResponse struct {
	ID             string              `json:"id"`
	SupplierID     string              `json:"supplierId"`
	BranchID       *string             `json:"branchId"`
	CFDIUUID       string              `json:"cfdiUuid"`
	Series         string              `json:"series"`
	Folio          string              `json:"folio"`
	IssuedAt       string              `json:"issuedAt"`
	Currency       string              `json:"currency"`
	ExchangeRate   *string             `json:"exchangeRate"`
	Subtotal       string              `json:"subtotal"`
	Discount       string              `json:"discount"`
	TaxTransferred string              `json:"taxTransferred"`
	TaxWithheld    string              `json:"taxWithheld"`
	Total          string              `json:"total"`
	IssuerTaxID    string              `json:"issuerTaxId"`
	IssuerName     string              `json:"issuerName"`
	XML            xmlDocumentResponse `json:"xml"`
	ImportedAt     string              `json:"importedAt"`
	CreatedAt      string              `json:"createdAt"`
	UpdatedAt      string              `json:"updatedAt"`
}

// xmlDocumentResponse omits Content: the raw XML bytes are never re-served
// over this API, only their hash and original filename for traceability.
type xmlDocumentResponse struct {
	Hash     string `json:"hash"`
	Filename string `json:"filename"`
}

type purchaseLineResponse struct {
	ID                 string  `json:"id"`
	PurchaseID         string  `json:"purchaseId"`
	LineNumber         int     `json:"lineNumber"`
	Description        string  `json:"description"`
	SupplierSKU        string  `json:"supplierSku"`
	SATProductCode     string  `json:"satProductCode"`
	Quantity           string  `json:"quantity"`
	UnitCode           string  `json:"unitCode"`
	Unit               string  `json:"unit"`
	UnitPrice          string  `json:"unitPrice"`
	Amount             string  `json:"amount"`
	Discount           string  `json:"discount"`
	TaxTransferred     string  `json:"taxTransferred"`
	TaxWithheld        string  `json:"taxWithheld"`
	TaxObject          string  `json:"taxObject"`
	SupplierProductID  *string `json:"supplierProductId"`
	ResolutionRevision string  `json:"resolutionRevision"`
	ResolutionOverride string  `json:"resolutionOverride"`
	EffectiveStatus    string  `json:"effectiveStatus"`
	EffectiveCause     string  `json:"effectiveCause"`
}

type purchaseLineWorkbenchRowResponse struct {
	LineID                string  `json:"lineId"`
	PurchaseID            string  `json:"purchaseId"`
	LineNumber            int     `json:"lineNumber"`
	IssuedAt              string  `json:"issuedAt"`
	Series                string  `json:"series"`
	Folio                 string  `json:"folio"`
	CFDIUUID              string  `json:"cfdiUuid"`
	SupplierID            string  `json:"supplierId"`
	SupplierDisplayName   string  `json:"supplierDisplayName"`
	Description           string  `json:"description"`
	SupplierSKU           string  `json:"supplierSku"`
	CommercialSupplierSKU *string `json:"commercialSupplierSku"`
	SATProductCode        string  `json:"satProductCode"`
	Quantity              string  `json:"quantity"`
	UnitCode              string  `json:"unitCode"`
	Unit                  string  `json:"unit"`
	UnitPrice             string  `json:"unitPrice"`
	Amount                string  `json:"amount"`
	Currency              string  `json:"currency"`
	SupplierProductID     *string `json:"supplierProductId"`
	MappingRevision       *string `json:"mappingRevision"`
	ResolutionRevision    string  `json:"resolutionRevision"`
	ResourceID            *string `json:"resourceId"`
	ResourceIdentity      *string `json:"resourceIdentity"`
	ResourceDisplayName   *string `json:"resourceDisplayName"`
	ResolutionOverride    string  `json:"resolutionOverride"`
	EffectiveStatus       string  `json:"effectiveStatus"`
	EffectiveCause        string  `json:"effectiveCause"`
}

type supplierProductResponse struct {
	ID              string  `json:"id"`
	SupplierID      string  `json:"supplierId"`
	SupplierSKU     string  `json:"supplierSku"`
	Description     string  `json:"description"`
	ResourceID      *string `json:"resourceId"`
	MappingRevision string  `json:"mappingRevision"`
	ResourceActive  *bool   `json:"resourceActive"`
	MappingState    string  `json:"mappingState"`
	MappingCause    string  `json:"mappingCause"`
	Notes           string  `json:"notes"`
	CreatedAt       string  `json:"createdAt"`
	UpdatedAt       string  `json:"updatedAt"`
}

type purchaseLineHistoryResponse struct {
	Line            purchaseLineResponse    `json:"line"`
	SupplierProduct supplierProductResponse `json:"supplierProduct"`
	PurchaseID      string                  `json:"purchaseId"`
	PurchaseUUID    string                  `json:"purchaseUuid"`
	SupplierID      string                  `json:"supplierId"`
	BranchID        *string                 `json:"branchId"`
	IssuedAt        string                  `json:"issuedAt"`
	Currency        string                  `json:"currency"`
}

func mapPurchase(p purchasecore.Purchase) purchaseResponse {
	return purchaseResponse{
		ID:             strconv.FormatInt(p.ID, 10),
		SupplierID:     strconv.FormatInt(p.SupplierID, 10),
		BranchID:       formatOptionalID(p.BranchID),
		CFDIUUID:       p.CFDIUUID,
		Series:         p.Series,
		Folio:          p.Folio,
		IssuedAt:       formatCFDIDate(p.IssuedAt),
		Currency:       p.Currency,
		ExchangeRate:   p.ExchangeRate,
		Subtotal:       p.Subtotal,
		Discount:       p.Discount,
		TaxTransferred: p.TaxTransferred,
		TaxWithheld:    p.TaxWithheld,
		Total:          p.Total,
		IssuerTaxID:    p.IssuerTaxID,
		IssuerName:     p.IssuerName,
		XML:            xmlDocumentResponse{Hash: p.XML.Hash, Filename: p.XML.Filename},
		ImportedAt:     p.ImportedAt.UTC().Format(time.RFC3339Nano),
		CreatedAt:      p.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:      p.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func mapPurchaseLine(l purchasecore.PurchaseLine) purchaseLineResponse {
	return purchaseLineResponse{
		ID:                 strconv.FormatInt(l.ID, 10),
		PurchaseID:         strconv.FormatInt(l.PurchaseID, 10),
		LineNumber:         l.LineNumber,
		Description:        l.Description,
		SupplierSKU:        l.SupplierSKU,
		SATProductCode:     l.SATProductCode,
		Quantity:           l.Quantity,
		UnitCode:           l.UnitCode,
		Unit:               l.Unit,
		UnitPrice:          l.UnitPrice,
		Amount:             l.Amount,
		Discount:           l.Discount,
		TaxTransferred:     l.TaxTransferred,
		TaxWithheld:        l.TaxWithheld,
		TaxObject:          l.TaxObject,
		SupplierProductID:  formatOptionalID(l.SupplierProductID),
		ResolutionRevision: strconv.FormatUint(uint64(l.ResolutionRevision), 10),
		ResolutionOverride: string(l.ResolutionOverride),
		EffectiveStatus:    string(l.EffectiveStatus),
		EffectiveCause:     string(l.EffectiveCause),
	}
}

func mapPurchaseLines(lines []purchasecore.PurchaseLine) []purchaseLineResponse {
	out := make([]purchaseLineResponse, len(lines))
	for i, l := range lines {
		out[i] = mapPurchaseLine(l)
	}
	return out
}

func mapPurchaseLineWorkbenchRow(row purchasecore.PurchaseLineRow) purchaseLineWorkbenchRowResponse {
	return purchaseLineWorkbenchRowResponse{
		LineID:                strconv.FormatInt(row.LineID, 10),
		PurchaseID:            strconv.FormatInt(row.PurchaseID, 10),
		LineNumber:            row.LineNumber,
		IssuedAt:              formatCFDIDate(row.IssuedAt),
		Series:                row.Series,
		Folio:                 row.Folio,
		CFDIUUID:              row.CFDIUUID,
		SupplierID:            strconv.FormatInt(row.SupplierID, 10),
		SupplierDisplayName:   row.SupplierDisplayName,
		Description:           row.Description,
		SupplierSKU:           row.SupplierSKU,
		CommercialSupplierSKU: row.CommercialSupplierSKU,
		SATProductCode:        row.SATProductCode,
		Quantity:              row.Quantity,
		UnitCode:              row.UnitCode,
		Unit:                  row.Unit,
		UnitPrice:             row.UnitPrice,
		Amount:                row.Amount,
		Currency:              row.Currency,
		SupplierProductID:     formatOptionalID(row.SupplierProductID),
		MappingRevision:       formatOptionalMappingRevision(row.MappingRevision),
		ResolutionRevision:    strconv.FormatUint(uint64(row.ResolutionRevision), 10),
		ResourceID:            formatOptionalID(row.ResourceID),
		ResourceIdentity:      row.ResourceIdentity,
		ResourceDisplayName:   row.ResourceDisplayName,
		ResolutionOverride:    string(row.ResolutionOverride),
		EffectiveStatus:       string(row.EffectiveStatus),
		EffectiveCause:        string(row.EffectiveCause),
	}
}

func mapSupplierProduct(sp purchasecore.SupplierProduct) supplierProductResponse {
	return supplierProductResponse{
		ID:              strconv.FormatInt(sp.ID, 10),
		SupplierID:      strconv.FormatInt(sp.SupplierID, 10),
		SupplierSKU:     sp.SupplierSKU,
		Description:     sp.Description,
		ResourceID:      formatOptionalID(sp.CurrentMapping.ResourceID),
		MappingRevision: strconv.FormatUint(uint64(sp.MappingRevision), 10),
		ResourceActive:  sp.ResourceActive,
		MappingState:    string(sp.MappingState),
		MappingCause:    string(sp.MappingCause),
		Notes:           sp.Notes,
		CreatedAt:       sp.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:       sp.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func mapPurchaseLineHistory(h purchasecore.PurchaseLineHistory) purchaseLineHistoryResponse {
	return purchaseLineHistoryResponse{
		Line:            mapPurchaseLine(h.Line),
		SupplierProduct: mapSupplierProduct(h.SupplierProduct),
		PurchaseID:      strconv.FormatInt(h.PurchaseID, 10),
		PurchaseUUID:    h.PurchaseUUID,
		SupplierID:      strconv.FormatInt(h.SupplierID, 10),
		BranchID:        formatOptionalID(h.BranchID),
		IssuedAt:        formatCFDIDate(h.IssuedAt),
		Currency:        h.Currency,
	}
}

func formatOptionalID(id *int64) *string {
	if id == nil {
		return nil
	}
	text := strconv.FormatInt(*id, 10)
	return &text
}

func formatOptionalMappingRevision(revision *purchasecore.MappingRevision) *string {
	if revision == nil {
		return nil
	}
	text := strconv.FormatUint(uint64(*revision), 10)
	return &text
}

func writePurchaseError(w http.ResponseWriter, err error) {
	status, message := purchaseError(err)
	code := purchasecore.Code(err)
	log.Printf("purchase error: code=%s status=%d message=%q", code, status, err.Error())
	resp := errorResponse{Error: message, Code: string(code)}
	if isPurchaseValidationCode(code) {
		resp.Detail = err.Error()
	}
	writeJSON(w, status, resp)
}

// isPurchaseValidationCode reports whether code's message is always a
// static, developer-authored string safe to return verbatim to the client.
func isPurchaseValidationCode(code purchasecore.ErrorCode) bool {
	switch code {
	case purchasecore.InvalidArgument, purchasecore.Validation,
		purchasecore.ResourceInactive, purchasecore.CommercialSupplierSKURequired,
		purchasecore.CommercialSupplierSKUForbidden:
		return true
	default:
		return false
	}
}

func purchaseError(err error) (int, string) {
	switch purchasecore.Code(err) {
	case purchasecore.InvalidArgument:
		return http.StatusBadRequest, "invalid request"
	case purchasecore.NotFound, purchasecore.PurchaseLineNotFound, purchasecore.ResourceNotFound:
		return http.StatusNotFound, "not found"
	case purchasecore.Conflict, purchasecore.PurchaseLineStateConflict,
		purchasecore.StaleResolutionRevision, purchasecore.StaleMappingRevision,
		purchasecore.SupplierProductTargetConflict, purchasecore.InvalidMappingTransition,
		purchasecore.IntegrityConflict:
		return http.StatusConflict, "conflict"
	case purchasecore.Validation, purchasecore.ResourceInactive,
		purchasecore.CommercialSupplierSKURequired, purchasecore.CommercialSupplierSKUForbidden:
		return http.StatusUnprocessableEntity, "validation failed"
	case purchasecore.Internal:
		return http.StatusInternalServerError, "internal server error"
	default:
		return http.StatusInternalServerError, "internal server error"
	}
}
