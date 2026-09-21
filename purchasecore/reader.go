package purchasecore

import (
	"context"
	"strings"
)

// ReadCapabilities is the service-shaped seam a Reader delegates to.
// Implementations are expected to be authoritative for the results they
// return and to translate their own internal errors to public Error
// values.
type ReadCapabilities interface {
	GetPurchase(ctx context.Context, id int64) (Purchase, error)
	GetPurchaseByUUID(ctx context.Context, uuid string) (Purchase, error)
	ListPurchaseLines(ctx context.Context, purchaseID int64) ([]PurchaseLine, error)
	ListPurchaseLinesWorkbench(ctx context.Context, q PurchaseLineQuery) (PurchaseLinePage, error)
	ListPurchasesBySupplier(ctx context.Context, supplierID int64, q ListCriteria) (PurchasePage, error)
	GetSupplierProduct(ctx context.Context, id int64) (SupplierProduct, error)
	FindSupplierProduct(ctx context.Context, supplierID int64, sku string) (SupplierProduct, error)
	ListSupplierProducts(ctx context.Context, supplierID int64, q ListCriteria) (SupplierProductPage, error)
	ListPurchaseLinesByResource(ctx context.Context, resourceID int64, q ListCriteria) (PurchaseLineHistoryPage, error)
	ListMappingAudit(ctx context.Context, supplierProductID int64, q ListCriteria) (MappingAuditPage, error)
}

// Reader is the public read-only Purchase and Price History contract. It
// validates request shape, delegates reads to ReadCapabilities, and
// defensively copies every returned value before exposing it.
type Reader struct {
	cap ReadCapabilities
}

// NewReadOnly returns a Reader backed by cap. A nil capability is rejected
// with INVALID_ARGUMENT.
func NewReadOnly(cap ReadCapabilities) (*Reader, error) {
	if cap == nil {
		return nil, NewError(InvalidArgument, "read capabilities are required")
	}
	return &Reader{cap: cap}, nil
}

// GetPurchase returns one purchase by id.
func (r *Reader) GetPurchase(ctx context.Context, id int64) (Purchase, error) {
	if id <= 0 {
		return Purchase{}, NewError(InvalidArgument, "purchase id must be positive")
	}
	p, err := r.cap.GetPurchase(ctx, id)
	if err != nil {
		return Purchase{}, err
	}
	return ClonePurchase(p), nil
}

// GetPurchaseByUUID returns the purchase carrying the given CFDI fiscal
// UUID, or a NOT_FOUND error when none does.
func (r *Reader) GetPurchaseByUUID(ctx context.Context, uuid string) (Purchase, error) {
	if strings.TrimSpace(uuid) == "" {
		return Purchase{}, NewError(InvalidArgument, "cfdi uuid must not be blank")
	}
	p, err := r.cap.GetPurchaseByUUID(ctx, uuid)
	if err != nil {
		return Purchase{}, err
	}
	return ClonePurchase(p), nil
}

// ListPurchaseLines returns every line of one purchase, in original order.
func (r *Reader) ListPurchaseLines(ctx context.Context, purchaseID int64) ([]PurchaseLine, error) {
	if purchaseID <= 0 {
		return nil, NewError(InvalidArgument, "purchase id must be positive")
	}
	lines, err := r.cap.ListPurchaseLines(ctx, purchaseID)
	if err != nil {
		return nil, err
	}
	return clonePurchaseLineSlice(lines), nil
}

// ListPurchaseLinesWorkbench returns a page of all purchase lines matching
// the supplied read-only workbench query.
func (r *Reader) ListPurchaseLinesWorkbench(ctx context.Context, q PurchaseLineQuery) (PurchaseLinePage, error) {
	if err := validatePurchaseLineQuery(q); err != nil {
		return PurchaseLinePage{}, err
	}
	page, err := r.cap.ListPurchaseLinesWorkbench(ctx, q)
	if err != nil {
		return PurchaseLinePage{}, err
	}
	page.Query = ClonePurchaseLineQuery(page.Query)
	page.Rows = clonePurchaseLineRowSlice(page.Rows)
	return page, nil
}

// ListPurchasesBySupplier answers "what have we bought from this
// supplier", most recent purchase first.
func (r *Reader) ListPurchasesBySupplier(ctx context.Context, supplierID int64, q ListCriteria) (PurchasePage, error) {
	if supplierID <= 0 {
		return PurchasePage{}, NewError(InvalidArgument, "supplier id must be positive")
	}
	page, err := r.cap.ListPurchasesBySupplier(ctx, supplierID, q)
	if err != nil {
		return PurchasePage{}, err
	}
	page.Purchases = clonePurchaseSlice(page.Purchases)
	return page, nil
}

// GetSupplierProduct returns one supplier product by id.
func (r *Reader) GetSupplierProduct(ctx context.Context, id int64) (SupplierProduct, error) {
	if id <= 0 {
		return SupplierProduct{}, NewError(InvalidArgument, "supplier product id must be positive")
	}
	sp, err := r.cap.GetSupplierProduct(ctx, id)
	if err != nil {
		return SupplierProduct{}, err
	}
	return CloneSupplierProduct(sp), nil
}

// FindSupplierProduct returns the supplier product identified by
// (supplierID, sku), or a NOT_FOUND error when none does.
func (r *Reader) FindSupplierProduct(ctx context.Context, supplierID int64, sku string) (SupplierProduct, error) {
	if supplierID <= 0 {
		return SupplierProduct{}, NewError(InvalidArgument, "supplier id must be positive")
	}
	if strings.TrimSpace(sku) == "" {
		return SupplierProduct{}, NewError(InvalidArgument, "sku must not be blank")
	}
	sp, err := r.cap.FindSupplierProduct(ctx, supplierID, sku)
	if err != nil {
		return SupplierProduct{}, err
	}
	return CloneSupplierProduct(sp), nil
}

// ListSupplierProducts returns a page of one supplier's registered
// products.
func (r *Reader) ListSupplierProducts(ctx context.Context, supplierID int64, q ListCriteria) (SupplierProductPage, error) {
	if supplierID <= 0 {
		return SupplierProductPage{}, NewError(InvalidArgument, "supplier id must be positive")
	}
	page, err := r.cap.ListSupplierProducts(ctx, supplierID, q)
	if err != nil {
		return SupplierProductPage{}, err
	}
	page.Products = cloneSupplierProductSlice(page.Products)
	return page, nil
}

// ListPurchaseLinesByResource answers, from a Resource Master entry, its
// full purchase history: which suppliers and supplier products have sold
// it, when, at what price and quantity, in what currency, and from which
// branch and source document. Most recent purchase first.
func (r *Reader) ListPurchaseLinesByResource(ctx context.Context, resourceID int64, q ListCriteria) (PurchaseLineHistoryPage, error) {
	if resourceID <= 0 {
		return PurchaseLineHistoryPage{}, NewError(InvalidArgument, "resource id must be positive")
	}
	page, err := r.cap.ListPurchaseLinesByResource(ctx, resourceID, q)
	if err != nil {
		return PurchaseLineHistoryPage{}, err
	}
	page.History = clonePurchaseLineHistorySlice(page.History)
	return page, nil
}

// ListMappingAudit returns the append-only confirmed mapping history.
func validatePurchaseLineQuery(q PurchaseLineQuery) error {
	if q.Limit < 0 || q.Limit > 50 {
		return NewError(InvalidArgument, "limit must be between 1 and 50, or zero for the default")
	}
	if q.Offset < 0 {
		return NewError(InvalidArgument, "offset must not be negative")
	}
	if q.SupplierID != nil && *q.SupplierID <= 0 {
		return NewError(InvalidArgument, "supplier id must be positive when provided")
	}
	if q.DateFrom != nil && q.DateFrom.IsZero() {
		return NewError(InvalidArgument, "date from must be valid when provided")
	}
	if q.DateTo != nil && q.DateTo.IsZero() {
		return NewError(InvalidArgument, "date to must be valid when provided")
	}
	if q.DateFrom != nil && q.DateTo != nil && q.DateFrom.After(*q.DateTo) {
		return NewError(InvalidArgument, "date from must not be after date to")
	}
	switch q.EffectiveStatus {
	case "", LinkPending, LinkLinked, LinkSuspended, LinkNotApplicable, LinkConflict:
		return nil
	default:
		return NewError(InvalidArgument, "invalid effective status")
	}
}

func (r *Reader) ListMappingAudit(ctx context.Context, supplierProductID int64, q ListCriteria) (MappingAuditPage, error) {
	if supplierProductID <= 0 {
		return MappingAuditPage{}, NewError(InvalidArgument, "supplier product id must be positive")
	}
	page, err := r.cap.ListMappingAudit(ctx, supplierProductID, q)
	if err != nil {
		return MappingAuditPage{}, err
	}
	page.Entries = cloneMappingAuditSlice(page.Entries)
	return page, nil
}
