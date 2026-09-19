package purchasecore

import (
	"context"
	"strings"
)

// WriteCapabilities is the service-shaped write seam a Writer delegates
// to.
type WriteCapabilities interface {
	ImportPurchase(ctx context.Context, req ImportRequest) (ImportResult, error)
	LinkSupplierProductToResource(ctx context.Context, req LinkSupplierProductRequest) (SupplierProduct, error)
	UnlinkSupplierProduct(ctx context.Context, req UnlinkSupplierProductRequest) (SupplierProduct, error)
	SetPurchaseLineLinkStatus(ctx context.Context, req SetPurchaseLineLinkStatusRequest) (PurchaseLine, error)
}

// Writer is the public write-only Purchase and Price History contract. It
// validates request shape and delegates writes to WriteCapabilities.
type Writer struct {
	cap WriteCapabilities
}

// NewWriter returns a Writer backed by cap. A nil capability is rejected
// with INVALID_ARGUMENT.
func NewWriter(cap WriteCapabilities) (*Writer, error) {
	if cap == nil {
		return nil, NewError(InvalidArgument, "write capabilities are required")
	}
	return &Writer{cap: cap}, nil
}

// ImportPurchase registers one CFDI 4.0 purchase document. It is
// idempotent on the document's fiscal UUID: re-importing the same document
// returns the existing purchase with ImportResult.AlreadyExisted set, and
// importing the same UUID with different relevant content returns a
// CONFLICT error without overwriting anything. Shape validation checks
// only that Actor is non-blank and XML is non-empty; every other rule —
// requiring a fiscal stamp, resolving or minimally registering the issuing
// supplier, validating a claimed branch — is the internal service's sole
// authority.
func (w *Writer) ImportPurchase(ctx context.Context, req ImportRequest) (ImportResult, error) {
	if err := requireActor(req.Actor); err != nil {
		return ImportResult{}, err
	}
	if len(req.XML) == 0 {
		return ImportResult{}, NewError(InvalidArgument, "xml content is required")
	}
	result, err := w.cap.ImportPurchase(ctx, req)
	if err != nil {
		return ImportResult{}, err
	}
	result.Purchase = ClonePurchase(result.Purchase)
	result.Lines = clonePurchaseLineSlice(result.Lines)
	return result, nil
}

// LinkSupplierProductToResource relates an existing supplier product to a
// Resource Master entry. It never modifies any PurchaseLine's original
// data: every PENDIENTE line already referencing the supplier product
// becomes VINCULADO, and every future purchase line resolving to the same
// supplier product reuses this relation automatically.
func (w *Writer) LinkSupplierProductToResource(ctx context.Context, req LinkSupplierProductRequest) (SupplierProduct, error) {
	if err := requireActor(req.Actor); err != nil {
		return SupplierProduct{}, err
	}
	if req.SupplierProductID <= 0 {
		return SupplierProduct{}, NewError(InvalidArgument, "supplier product id must be positive")
	}
	if req.ResourceID <= 0 {
		return SupplierProduct{}, NewError(InvalidArgument, "resource id must be positive")
	}
	sp, err := w.cap.LinkSupplierProductToResource(ctx, req)
	if err != nil {
		return SupplierProduct{}, err
	}
	return CloneSupplierProduct(sp), nil
}

// UnlinkSupplierProduct removes a supplier product's Resource Master
// relation, correcting a previous assignment without touching any purchase
// line's original data. Every VINCULADO line referencing it reverts to
// PENDIENTE.
func (w *Writer) UnlinkSupplierProduct(ctx context.Context, req UnlinkSupplierProductRequest) (SupplierProduct, error) {
	if err := requireActor(req.Actor); err != nil {
		return SupplierProduct{}, err
	}
	if req.SupplierProductID <= 0 {
		return SupplierProduct{}, NewError(InvalidArgument, "supplier product id must be positive")
	}
	sp, err := w.cap.UnlinkSupplierProduct(ctx, req)
	if err != nil {
		return SupplierProduct{}, err
	}
	return CloneSupplierProduct(sp), nil
}

// SetPurchaseLineLinkStatus manually overrides one line's linking state,
// for the cases automatic resolution must not decide on its own: marking a
// line NO_APLICA, or resolving a CONFLICTO a human has reviewed.
func (w *Writer) SetPurchaseLineLinkStatus(ctx context.Context, req SetPurchaseLineLinkStatusRequest) (PurchaseLine, error) {
	if err := requireActor(req.Actor); err != nil {
		return PurchaseLine{}, err
	}
	if req.PurchaseLineID <= 0 {
		return PurchaseLine{}, NewError(InvalidArgument, "purchase line id must be positive")
	}
	switch req.Status {
	case LinkPending, LinkLinked, LinkNotApplicable, LinkConflict:
	default:
		return PurchaseLine{}, NewError(InvalidArgument, "link status is not a recognized value")
	}
	line, err := w.cap.SetPurchaseLineLinkStatus(ctx, req)
	if err != nil {
		return PurchaseLine{}, err
	}
	return ClonePurchaseLine(line), nil
}

func requireActor(actor string) error {
	if strings.TrimSpace(actor) == "" {
		return NewError(InvalidArgument, "actor is required")
	}
	return nil
}
