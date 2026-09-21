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
	ListMappingAudit(ctx context.Context, supplierProductID int64, criteria domain.ListCriteria) ([]domain.MappingAuditEntry, error)
	ConfirmMapping(ctx context.Context, command domain.ConfirmMappingCommand) (domain.SupplierProduct, error)
	CorrectMapping(ctx context.Context, command domain.CorrectMappingCommand) (domain.SupplierProduct, error)
	ExceptionalUnlink(ctx context.Context, command domain.ExceptionalUnlinkCommand) (domain.SupplierProduct, error)
	ReportIdentityConflict(ctx context.Context, command domain.ReportIdentityConflictCommand) (domain.SupplierProduct, error)
	ResolveIdentityConflict(ctx context.Context, command domain.ResolveIdentityConflictCommand) (domain.SupplierProduct, error)
	MarkNotApplicable(ctx context.Context, lineID int64) (domain.PurchaseLine, error)
	MarkConflict(ctx context.Context, lineID int64) (domain.PurchaseLine, error)
	ClearOverride(ctx context.Context, lineID int64) (domain.PurchaseLine, error)
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
	return public.PurchaseLineHistoryPage{Query: public.ListCriteria{Limit: limit, Offset: q.Offset}, History: mapPurchaseLineHistorySlice(history[:end]), HasPrevious: q.Offset > 0, HasNext: hasNext}, nil
}

func (a *Adapter) ListMappingAudit(ctx context.Context, supplierProductID int64, q public.ListCriteria) (public.MappingAuditPage, error) {
	limit := effectiveLimit(q.Limit)
	entries, err := a.service.ListMappingAudit(ctx, supplierProductID, domain.ListCriteria{Limit: limit + 1, Offset: q.Offset})
	if err != nil {
		return public.MappingAuditPage{}, mapError(err)
	}
	end, hasNext := trimToPage(len(entries), limit)
	mapped := make([]public.MappingAuditEntry, end)
	for i := range mapped {
		mapped[i] = mapMappingAuditEntry(entries[i])
	}
	return public.MappingAuditPage{Query: public.ListCriteria{Limit: limit, Offset: q.Offset}, Entries: mapped, HasPrevious: q.Offset > 0, HasNext: hasNext}, nil
}

func decisionMetadata(decision public.MappingDecisionMetadata) domain.MappingDecisionMetadata {
	return domain.MappingDecisionMetadata{Actor: decision.Actor, Origin: domain.MappingOrigin(decision.Origin), Reason: decision.Reason, At: decision.At}
}

func (a *Adapter) ConfirmMapping(ctx context.Context, req public.ConfirmMappingRequest) (public.SupplierProduct, error) {
	sp, err := a.service.ConfirmMapping(ctx, domain.ConfirmMappingCommand{SupplierProductID: req.SupplierProductID, ResourceID: req.ResourceID, ExpectedRevision: domain.MappingRevision(req.ExpectedRevision), Decision: decisionMetadata(req.Decision)})
	if err != nil {
		return public.SupplierProduct{}, mapError(err)
	}
	return mapSupplierProduct(sp), nil
}

func (a *Adapter) CorrectMapping(ctx context.Context, req public.CorrectMappingRequest) (public.SupplierProduct, error) {
	sp, err := a.service.CorrectMapping(ctx, domain.CorrectMappingCommand{SupplierProductID: req.SupplierProductID, ExpectedCurrentResource: req.ExpectedCurrentResourceID, ResourceID: req.ResourceID, ExpectedRevision: domain.MappingRevision(req.ExpectedRevision), Decision: decisionMetadata(req.Decision)})
	if err != nil {
		return public.SupplierProduct{}, mapError(err)
	}
	return mapSupplierProduct(sp), nil
}

func (a *Adapter) ExceptionalUnlink(ctx context.Context, req public.ExceptionalUnlinkRequest) (public.SupplierProduct, error) {
	sp, err := a.service.ExceptionalUnlink(ctx, domain.ExceptionalUnlinkCommand{SupplierProductID: req.SupplierProductID, ExpectedCurrentResource: req.ExpectedCurrentResourceID, ExpectedRevision: domain.MappingRevision(req.ExpectedRevision), Decision: decisionMetadata(req.Decision)})
	if err != nil {
		return public.SupplierProduct{}, mapError(err)
	}
	return mapSupplierProduct(sp), nil
}

func (a *Adapter) ReportIdentityConflict(ctx context.Context, req public.ReportIdentityConflictRequest) (public.SupplierProduct, error) {
	sp, err := a.service.ReportIdentityConflict(ctx, domain.ReportIdentityConflictCommand{SupplierProductID: req.SupplierProductID, ExpectedCurrentResource: req.ExpectedCurrentResourceID, ExpectedRevision: domain.MappingRevision(req.ExpectedRevision), Decision: decisionMetadata(req.Decision)})
	if err != nil {
		return public.SupplierProduct{}, mapError(err)
	}
	return mapSupplierProduct(sp), nil
}

func (a *Adapter) ResolveIdentityConflict(ctx context.Context, req public.ResolveIdentityConflictRequest) (public.SupplierProduct, error) {
	sp, err := a.service.ResolveIdentityConflict(ctx, domain.ResolveIdentityConflictCommand{SupplierProductID: req.SupplierProductID, ExpectedCurrentResource: req.ExpectedCurrentResourceID, ResourceID: req.ResourceID, ExpectedRevision: domain.MappingRevision(req.ExpectedRevision), Decision: decisionMetadata(req.Decision)})
	if err != nil {
		return public.SupplierProduct{}, mapError(err)
	}
	return mapSupplierProduct(sp), nil
}

func (a *Adapter) MarkNotApplicable(ctx context.Context, lineID int64) (public.PurchaseLine, error) {
	line, err := a.service.MarkNotApplicable(ctx, lineID)
	if err != nil {
		return public.PurchaseLine{}, mapError(err)
	}
	return mapPurchaseLine(line), nil
}
func (a *Adapter) MarkConflict(ctx context.Context, lineID int64) (public.PurchaseLine, error) {
	line, err := a.service.MarkConflict(ctx, lineID)
	if err != nil {
		return public.PurchaseLine{}, mapError(err)
	}
	return mapPurchaseLine(line), nil
}
func (a *Adapter) ClearOverride(ctx context.Context, lineID int64) (public.PurchaseLine, error) {
	line, err := a.service.ClearOverride(ctx, lineID)
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
		ID:                 l.ID,
		PurchaseID:         l.PurchaseID,
		LineNumber:         l.LineNumber,
		Description:        l.Description,
		SupplierSKU:        l.SupplierSKU,
		SATProductCode:     l.SATProductCode,
		Quantity:           l.Quantity.String(),
		UnitCode:           l.UnitCode,
		Unit:               l.Unit,
		UnitPrice:          l.UnitPrice.String(),
		Amount:             l.Amount.String(),
		Discount:           l.Discount.String(),
		TaxTransferred:     l.TaxTransferred.String(),
		TaxWithheld:        l.TaxWithheld.String(),
		TaxObject:          l.TaxObject,
		SupplierProductID:  copyInt64(l.SupplierProductID),
		ResolutionOverride: public.LinkStatus(l.ResolutionOverride),
		EffectiveStatus:    public.LinkStatus(l.DerivedStatus),
		EffectiveCause:     public.MappingCause(l.DerivedCause),
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
	active := true
	if sp.ResourceActive != nil {
		active = *sp.ResourceActive
	}
	return public.SupplierProduct{
		ID: sp.ID, SupplierID: sp.SupplierID, SupplierSKU: sp.SupplierSKU, Description: sp.Description,
		CurrentMapping:  public.SupplierProductMapping{ResourceID: copyInt64(sp.CurrentMapping.ResourceID), IdentityConflict: sp.CurrentMapping.IdentityConflict},
		MappingRevision: public.MappingRevision(sp.MappingRevision), ResourceActive: copyBool(sp.ResourceActive),
		MappingState: public.MappingState(sp.CurrentMapping.State(active)), MappingCause: public.MappingCause(sp.CurrentMapping.Cause(active)),
		Notes: sp.Notes, CreatedAt: sp.CreatedAt, UpdatedAt: sp.UpdatedAt,
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
	return public.PurchaseLineHistory{Line: mapPurchaseLine(h.Line), SupplierProduct: mapSupplierProduct(h.SupplierProduct), PurchaseID: h.PurchaseID, PurchaseUUID: h.PurchaseUUID, SupplierID: h.SupplierID, BranchID: copyInt64(h.BranchID), IssuedAt: h.IssuedAt, Currency: h.Currency}
}

func mapMappingAuditEntry(entry domain.MappingAuditEntry) public.MappingAuditEntry {
	return public.MappingAuditEntry{SupplierProductID: entry.SupplierProductID,
		PreviousMapping: public.SupplierProductMapping{ResourceID: copyInt64(entry.PreviousMapping.ResourceID), IdentityConflict: entry.PreviousMapping.IdentityConflict},
		NewMapping:      public.SupplierProductMapping{ResourceID: copyInt64(entry.NewMapping.ResourceID), IdentityConflict: entry.NewMapping.IdentityConflict},
		PreviousState:   public.MappingState(entry.PreviousState), NewState: public.MappingState(entry.NewState),
		PreviousRevision: public.MappingRevision(entry.PreviousRevision), NewRevision: public.MappingRevision(entry.NewRevision),
		Operation: public.MappingOperation(entry.Operation), Decision: public.MappingDecisionMetadata{Actor: entry.Decision.Actor, Origin: public.MappingOrigin(entry.Decision.Origin), Reason: entry.Decision.Reason, At: entry.Decision.At}}
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

func copyBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	v := *value
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
	case errors.Is(err, domain.ErrValidation), errors.Is(err, supplierdomain.ErrValidation), errors.Is(err, domain.ErrInvalidMappingTransition), errors.Is(err, domain.ErrResourceInactive):
		return public.NewError(public.Validation, "purchase master validation failed")
	case errors.Is(err, domain.ErrStaleMappingRevision), errors.Is(err, domain.ErrCommitAmbiguous):
		return public.NewError(public.Conflict, "purchase master mapping conflict")
	case errors.Is(err, domain.ErrResourceNotFound):
		return public.NewError(public.NotFound, "purchase master resource not found")
	default:
		return public.NewError(public.Internal, "purchase master operation failed")
	}
}
