// Package purchasecore is the internal bridge between the public
// purchasecore contract and the authoritative Purchase and Price History
// application service. It translates public requests to domain calls, maps
// internal errors to public errors, and defensively copies data crossing
// the boundary. It never re-implements a business rule already owned by
// the internal service — most notably CFDI parsing, fiscal-UUID
// deduplication, and supplier resolution, which this bridge only observes
// the result of.
package purchasecore

import (
	"context"
	"errors"

	"github.com/GARFEX33/garfex-costos-unitarios/internal/core"
	"github.com/GARFEX33/garfex-costos-unitarios/internal/modules/purchases/app"
	"github.com/GARFEX33/garfex-costos-unitarios/internal/modules/purchases/domain"
	supplierdomain "github.com/GARFEX33/garfex-costos-unitarios/internal/modules/suppliers/domain"
	public "github.com/GARFEX33/garfex-costos-unitarios/purchasecore"
	"github.com/shopspring/decimal"
)

// defaultLimit matches the internal postgres repository's own private
// default (internal/modules/purchases/postgres/repository.go). Declared
// again here because the bridge must resolve a query's effective limit
// itself, before deciding how to over-fetch for pagination.
const defaultLimit = 100

// service is the narrow seam this bridge depends on: exactly the methods
// public.ReadCapabilities and public.WriteCapabilities need, out of
// purchases/app.Service's full surface.
type service interface {
	ImportCFDI(ctx context.Context, xml []byte, opts app.ImportOptions) (domain.ImportResult, error)
	GetPurchase(ctx context.Context, id int64) (domain.Purchase, error)
	GetPurchaseByUUID(ctx context.Context, uuid string) (domain.Purchase, error)
	ListPurchaseLines(ctx context.Context, purchaseID int64) ([]domain.PurchaseLine, error)
	ListPurchasesBySupplier(ctx context.Context, supplierID int64, criteria domain.ListCriteria) ([]domain.Purchase, error)
	GetSupplierProduct(ctx context.Context, id int64) (domain.SupplierProduct, error)
	FindSupplierProduct(ctx context.Context, supplierID int64, sku string) (domain.SupplierProduct, error)
	ListSupplierProducts(ctx context.Context, supplierID int64, criteria domain.ListCriteria) ([]domain.SupplierProduct, error)
	ListPurchaseLinesByResource(ctx context.Context, resourceID int64, criteria domain.ListCriteria) ([]domain.PurchaseLineHistory, error)
	LinkSupplierProductToResource(ctx context.Context, supplierProductID, resourceID int64) (domain.SupplierProduct, error)
	UnlinkSupplierProduct(ctx context.Context, supplierProductID int64) (domain.SupplierProduct, error)
	SetPurchaseLineLinkStatus(ctx context.Context, lineID int64, status domain.LinkStatus) (domain.PurchaseLine, error)
}

// Adapter implements public.ReadCapabilities and public.WriteCapabilities
// over an internal service.
type Adapter struct {
	service service
}

var _ public.ReadCapabilities = (*Adapter)(nil)
var _ public.WriteCapabilities = (*Adapter)(nil)

// NewAdapter returns a ReadCapabilities and WriteCapabilities
// implementation backed by svc. svc may not be nil.
func NewAdapter(svc service) *Adapter {
	return &Adapter{service: svc}
}

// ImportPurchase registers one CFDI purchase document. req.Actor is
// diagnostic-only attribution, carried via core.WithActor.
func (a *Adapter) ImportPurchase(ctx context.Context, req public.ImportRequest) (public.ImportResult, error) {
	ctx = core.WithActor(ctx, req.Actor)
	result, err := a.service.ImportCFDI(ctx, req.XML, app.ImportOptions{BranchID: req.BranchID, Filename: req.Filename})
	if err != nil {
		return public.ImportResult{}, mapError(err)
	}
	return public.ImportResult{
		Purchase:       mapPurchase(result.Purchase),
		Lines:          mapPurchaseLineSlice(result.Lines),
		AlreadyExisted: result.AlreadyExisted,
	}, nil
}

// GetPurchase returns one purchase by id.
func (a *Adapter) GetPurchase(ctx context.Context, id int64) (public.Purchase, error) {
	p, err := a.service.GetPurchase(ctx, id)
	if err != nil {
		return public.Purchase{}, mapError(err)
	}
	return mapPurchase(p), nil
}

// GetPurchaseByUUID returns the purchase carrying the given fiscal UUID.
func (a *Adapter) GetPurchaseByUUID(ctx context.Context, uuid string) (public.Purchase, error) {
	p, err := a.service.GetPurchaseByUUID(ctx, uuid)
	if err != nil {
		return public.Purchase{}, mapError(err)
	}
	return mapPurchase(p), nil
}

// ListPurchaseLines returns every line of one purchase, in original order.
func (a *Adapter) ListPurchaseLines(ctx context.Context, purchaseID int64) ([]public.PurchaseLine, error) {
	lines, err := a.service.ListPurchaseLines(ctx, purchaseID)
	if err != nil {
		return nil, mapError(err)
	}
	return mapPurchaseLineSlice(lines), nil
}

// ListPurchasesBySupplier returns a page of one supplier's purchase
// history, most recent first. The internal repository uses a plain LIMIT,
// not LIMIT+1, so this bridge over-fetches by one row itself to derive
// HasNext, the same technique suppliercore's SearchSuppliers uses.
func (a *Adapter) ListPurchasesBySupplier(ctx context.Context, supplierID int64, q public.ListCriteria) (public.PurchasePage, error) {
	limit := effectiveLimit(q.Limit)
	purchases, err := a.service.ListPurchasesBySupplier(ctx, supplierID, domain.ListCriteria{Limit: limit + 1, Offset: q.Offset})
	if err != nil {
		return public.PurchasePage{}, mapError(err)
	}
	end, hasNext := trimToPage(len(purchases), limit)
	return public.PurchasePage{
		Query:       public.ListCriteria{Limit: limit, Offset: q.Offset},
		Purchases:   mapPurchaseSlice(purchases[:end]),
		HasPrevious: q.Offset > 0,
		HasNext:     hasNext,
	}, nil
}

// GetSupplierProduct returns one supplier product by id.
func (a *Adapter) GetSupplierProduct(ctx context.Context, id int64) (public.SupplierProduct, error) {
	sp, err := a.service.GetSupplierProduct(ctx, id)
	if err != nil {
		return public.SupplierProduct{}, mapError(err)
	}
	return mapSupplierProduct(sp), nil
}

// FindSupplierProduct returns the supplier product identified by
// (supplierID, sku).
func (a *Adapter) FindSupplierProduct(ctx context.Context, supplierID int64, sku string) (public.SupplierProduct, error) {
	sp, err := a.service.FindSupplierProduct(ctx, supplierID, sku)
	if err != nil {
		return public.SupplierProduct{}, mapError(err)
	}
	return mapSupplierProduct(sp), nil
}

// ListSupplierProducts returns a page of one supplier's registered
// products. Same over-fetch strategy as ListPurchasesBySupplier.
func (a *Adapter) ListSupplierProducts(ctx context.Context, supplierID int64, q public.ListCriteria) (public.SupplierProductPage, error) {
	limit := effectiveLimit(q.Limit)
	products, err := a.service.ListSupplierProducts(ctx, supplierID, domain.ListCriteria{Limit: limit + 1, Offset: q.Offset})
	if err != nil {
		return public.SupplierProductPage{}, mapError(err)
	}
	end, hasNext := trimToPage(len(products), limit)
	return public.SupplierProductPage{
		Query:       public.ListCriteria{Limit: limit, Offset: q.Offset},
		Products:    mapSupplierProductSlice(products[:end]),
		HasPrevious: q.Offset > 0,
		HasNext:     hasNext,
	}, nil
}

// ListPurchaseLinesByResource returns a page of one Resource Master
// entry's purchase history, most recent first. Same over-fetch strategy as
// ListPurchasesBySupplier.
func (a *Adapter) ListPurchaseLinesByResource(ctx context.Context, resourceID int64, q public.ListCriteria) (public.PurchaseLineHistoryPage, error) {
	limit := effectiveLimit(q.Limit)
	history, err := a.service.ListPurchaseLinesByResource(ctx, resourceID, domain.ListCriteria{Limit: limit + 1, Offset: q.Offset})
	if err != nil {
		return public.PurchaseLineHistoryPage{}, mapError(err)
	}
	end, hasNext := trimToPage(len(history), limit)
	return public.PurchaseLineHistoryPage{
		Query:       public.ListCriteria{Limit: limit, Offset: q.Offset},
		History:     mapPurchaseLineHistorySlice(history[:end]),
		HasPrevious: q.Offset > 0,
		HasNext:     hasNext,
	}, nil
}

// LinkSupplierProductToResource relates a supplier product to a Resource
// Master entry. req.Actor is diagnostic-only attribution.
func (a *Adapter) LinkSupplierProductToResource(ctx context.Context, req public.LinkSupplierProductRequest) (public.SupplierProduct, error) {
	ctx = core.WithActor(ctx, req.Actor)
	sp, err := a.service.LinkSupplierProductToResource(ctx, req.SupplierProductID, req.ResourceID)
	if err != nil {
		return public.SupplierProduct{}, mapError(err)
	}
	return mapSupplierProduct(sp), nil
}

// UnlinkSupplierProduct removes a supplier product's Resource Master
// relation. req.Actor is diagnostic-only attribution.
func (a *Adapter) UnlinkSupplierProduct(ctx context.Context, req public.UnlinkSupplierProductRequest) (public.SupplierProduct, error) {
	ctx = core.WithActor(ctx, req.Actor)
	sp, err := a.service.UnlinkSupplierProduct(ctx, req.SupplierProductID)
	if err != nil {
		return public.SupplierProduct{}, mapError(err)
	}
	return mapSupplierProduct(sp), nil
}

// SetPurchaseLineLinkStatus manually overrides one line's linking state.
// req.Actor is diagnostic-only attribution.
func (a *Adapter) SetPurchaseLineLinkStatus(ctx context.Context, req public.SetPurchaseLineLinkStatusRequest) (public.PurchaseLine, error) {
	ctx = core.WithActor(ctx, req.Actor)
	line, err := a.service.SetPurchaseLineLinkStatus(ctx, req.PurchaseLineID, domain.LinkStatus(req.Status))
	if err != nil {
		return public.PurchaseLine{}, mapError(err)
	}
	return mapPurchaseLine(line), nil
}

func effectiveLimit(requested int) int {
	if requested <= 0 {
		return defaultLimit
	}
	return requested
}

func trimToPage(results int, limit int) (end int, hasNext bool) {
	if results > limit {
		return limit, true
	}
	return results, false
}

func mapPurchase(p domain.Purchase) public.Purchase {
	return public.Purchase{
		ID:             p.ID,
		SupplierID:     p.Supplier.SupplierID,
		BranchID:       copyInt64(p.Supplier.BranchID),
		CFDIUUID:       p.CFDIUUID,
		Series:         p.Series,
		Folio:          p.Folio,
		IssuedAt:       p.IssuedAt,
		Currency:       p.Currency,
		ExchangeRate:   decimalToStringPtr(p.ExchangeRate),
		Subtotal:       p.Subtotal.String(),
		Discount:       p.Discount.String(),
		TaxTransferred: p.TaxTransferred.String(),
		TaxWithheld:    p.TaxWithheld.String(),
		Total:          p.Total.String(),
		IssuerTaxID:    p.IssuerTaxID,
		IssuerName:     p.IssuerName,
		XML: public.XMLDocument{
			Content:  p.XML.Content,
			Hash:     p.XML.Hash,
			Filename: p.XML.Filename,
		},
		ImportedAt: p.ImportedAt,
		CreatedAt:  p.CreatedAt,
		UpdatedAt:  p.UpdatedAt,
	}
}

func mapPurchaseSlice(purchases []domain.Purchase) []public.Purchase {
	out := make([]public.Purchase, len(purchases))
	for i := range purchases {
		out[i] = mapPurchase(purchases[i])
	}
	return out
}

func mapPurchaseLine(l domain.PurchaseLine) public.PurchaseLine {
	return public.PurchaseLine{
		ID:                l.ID,
		PurchaseID:        l.PurchaseID,
		LineNumber:        l.LineNumber,
		Description:       l.Description,
		SupplierSKU:       l.SupplierSKU,
		SATProductCode:    l.SATProductCode,
		Quantity:          l.Quantity.String(),
		UnitCode:          l.UnitCode,
		Unit:              l.Unit,
		UnitPrice:         l.UnitPrice.String(),
		Amount:            l.Amount.String(),
		Discount:          l.Discount.String(),
		TaxTransferred:    l.TaxTransferred.String(),
		TaxWithheld:       l.TaxWithheld.String(),
		TaxObject:         l.TaxObject,
		SupplierProductID: copyInt64(l.SupplierProductID),
		LinkStatus:        public.LinkStatus(l.LinkStatus),
	}
}

func mapPurchaseLineSlice(lines []domain.PurchaseLine) []public.PurchaseLine {
	out := make([]public.PurchaseLine, len(lines))
	for i := range lines {
		out[i] = mapPurchaseLine(lines[i])
	}
	return out
}

func mapSupplierProduct(sp domain.SupplierProduct) public.SupplierProduct {
	return public.SupplierProduct{
		ID:          sp.ID,
		SupplierID:  sp.SupplierID,
		SupplierSKU: sp.SupplierSKU,
		Description: sp.Description,
		ResourceID:  copyInt64(sp.CurrentMapping.ResourceID),
		Notes:       sp.Notes,
		CreatedAt:   sp.CreatedAt,
		UpdatedAt:   sp.UpdatedAt,
	}
}

func mapSupplierProductSlice(products []domain.SupplierProduct) []public.SupplierProduct {
	out := make([]public.SupplierProduct, len(products))
	for i := range products {
		out[i] = mapSupplierProduct(products[i])
	}
	return out
}

func mapPurchaseLineHistory(h domain.PurchaseLineHistory) public.PurchaseLineHistory {
	return public.PurchaseLineHistory{
		Line:            mapPurchaseLine(h.Line),
		SupplierProduct: mapSupplierProduct(h.SupplierProduct),
		PurchaseID:      h.PurchaseID,
		PurchaseUUID:    h.PurchaseUUID,
		SupplierID:      h.SupplierID,
		BranchID:        copyInt64(h.BranchID),
		IssuedAt:        h.IssuedAt,
		Currency:        h.Currency,
	}
}

func mapPurchaseLineHistorySlice(history []domain.PurchaseLineHistory) []public.PurchaseLineHistory {
	out := make([]public.PurchaseLineHistory, len(history))
	for i := range history {
		out[i] = mapPurchaseLineHistory(history[i])
	}
	return out
}

func copyInt64(id *int64) *int64 {
	if id == nil {
		return nil
	}
	v := *id
	return &v
}

func decimalToStringPtr(d *decimal.Decimal) *string {
	if d == nil {
		return nil
	}
	s := d.String()
	return &s
}

// mapError classifies an internal Purchase and Price History error into a
// public Error. Import can also surface an error rooted in the Supplier
// Master domain (resolving or minimally registering the issuing supplier,
// or validating a claimed branch), so both taxonomies are checked. The
// default branch always carries a fixed, generic message — never the
// original error's text — so a raw, unsanitized internal error can never
// leak PostgreSQL detail through this bridge.
func mapError(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound), errors.Is(err, supplierdomain.ErrNotFound):
		return public.NewError(public.NotFound, "purchase master record not found")
	case errors.Is(err, domain.ErrConflict), errors.Is(err, supplierdomain.ErrConflict):
		return public.NewError(public.Conflict, "purchase master conflict")
	case errors.Is(err, domain.ErrValidation), errors.Is(err, supplierdomain.ErrValidation):
		return public.NewError(public.Validation, "purchase master validation failed")
	default:
		return public.NewError(public.Internal, "purchase master operation failed")
	}
}
