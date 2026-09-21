package purchasecore

import (
	"context"
	"strings"
)

// WriteCapabilities is the semantic Purchase Core write seam.
type WriteCapabilities interface {
	ImportPurchase(context.Context, ImportRequest) (ImportResult, error)
	ConfirmMapping(context.Context, ConfirmMappingRequest) (SupplierProduct, error)
	CorrectMapping(context.Context, CorrectMappingRequest) (SupplierProduct, error)
	ExceptionalUnlink(context.Context, ExceptionalUnlinkRequest) (SupplierProduct, error)
	ReportIdentityConflict(context.Context, ReportIdentityConflictRequest) (SupplierProduct, error)
	ResolveIdentityConflict(context.Context, ResolveIdentityConflictRequest) (SupplierProduct, error)
	MarkNotApplicable(context.Context, int64) (PurchaseLine, error)
	MarkConflict(context.Context, int64) (PurchaseLine, error)
	ClearOverride(context.Context, int64) (PurchaseLine, error)
}

type Writer struct{ cap WriteCapabilities }

func NewWriter(cap WriteCapabilities) (*Writer, error) {
	if cap == nil {
		return nil, NewError(InvalidArgument, "write capabilities are required")
	}
	return &Writer{cap: cap}, nil
}

func (w *Writer) ImportPurchase(ctx context.Context, req ImportRequest) (ImportResult, error) {
	if strings.TrimSpace(req.Actor) == "" {
		return ImportResult{}, NewError(InvalidArgument, "actor is required")
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

func (w *Writer) ConfirmMapping(ctx context.Context, req ConfirmMappingRequest) (SupplierProduct, error) {
	if err := validDecision(req.Decision); err != nil {
		return SupplierProduct{}, err
	}
	if req.SupplierProductID <= 0 || req.ResourceID <= 0 {
		return SupplierProduct{}, NewError(InvalidArgument, "mapping identifiers must be positive")
	}
	sp, err := w.cap.ConfirmMapping(ctx, req)
	if err != nil {
		return SupplierProduct{}, err
	}
	return CloneSupplierProduct(sp), nil
}

func (w *Writer) CorrectMapping(ctx context.Context, req CorrectMappingRequest) (SupplierProduct, error) {
	if err := validDecision(req.Decision); err != nil {
		return SupplierProduct{}, err
	}
	if req.SupplierProductID <= 0 || req.ExpectedCurrentResourceID <= 0 || req.ResourceID <= 0 {
		return SupplierProduct{}, NewError(InvalidArgument, "mapping identifiers must be positive")
	}
	sp, err := w.cap.CorrectMapping(ctx, req)
	if err != nil {
		return SupplierProduct{}, err
	}
	return CloneSupplierProduct(sp), nil
}

func (w *Writer) ExceptionalUnlink(ctx context.Context, req ExceptionalUnlinkRequest) (SupplierProduct, error) {
	if err := validDecision(req.Decision); err != nil {
		return SupplierProduct{}, err
	}
	if req.SupplierProductID <= 0 || req.ExpectedCurrentResourceID <= 0 {
		return SupplierProduct{}, NewError(InvalidArgument, "mapping identifiers must be positive")
	}
	sp, err := w.cap.ExceptionalUnlink(ctx, req)
	if err != nil {
		return SupplierProduct{}, err
	}
	return CloneSupplierProduct(sp), nil
}

func (w *Writer) ReportIdentityConflict(ctx context.Context, req ReportIdentityConflictRequest) (SupplierProduct, error) {
	if err := validDecision(req.Decision); err != nil {
		return SupplierProduct{}, err
	}
	if req.SupplierProductID <= 0 || req.ExpectedCurrentResourceID <= 0 {
		return SupplierProduct{}, NewError(InvalidArgument, "mapping identifiers must be positive")
	}
	sp, err := w.cap.ReportIdentityConflict(ctx, req)
	if err != nil {
		return SupplierProduct{}, err
	}
	return CloneSupplierProduct(sp), nil
}

func (w *Writer) ResolveIdentityConflict(ctx context.Context, req ResolveIdentityConflictRequest) (SupplierProduct, error) {
	if err := validDecision(req.Decision); err != nil {
		return SupplierProduct{}, err
	}
	if req.SupplierProductID <= 0 || req.ExpectedCurrentResourceID <= 0 || req.ResourceID <= 0 {
		return SupplierProduct{}, NewError(InvalidArgument, "mapping identifiers must be positive")
	}
	sp, err := w.cap.ResolveIdentityConflict(ctx, req)
	if err != nil {
		return SupplierProduct{}, err
	}
	return CloneSupplierProduct(sp), nil
}

func (w *Writer) MarkNotApplicable(ctx context.Context, lineID int64) (PurchaseLine, error) {
	if lineID <= 0 {
		return PurchaseLine{}, NewError(InvalidArgument, "purchase line id must be positive")
	}
	line, err := w.cap.MarkNotApplicable(ctx, lineID)
	if err != nil {
		return PurchaseLine{}, err
	}
	return ClonePurchaseLine(line), nil
}

func (w *Writer) MarkConflict(ctx context.Context, lineID int64) (PurchaseLine, error) {
	if lineID <= 0 {
		return PurchaseLine{}, NewError(InvalidArgument, "purchase line id must be positive")
	}
	line, err := w.cap.MarkConflict(ctx, lineID)
	if err != nil {
		return PurchaseLine{}, err
	}
	return ClonePurchaseLine(line), nil
}

func (w *Writer) ClearOverride(ctx context.Context, lineID int64) (PurchaseLine, error) {
	if lineID <= 0 {
		return PurchaseLine{}, NewError(InvalidArgument, "purchase line id must be positive")
	}
	line, err := w.cap.ClearOverride(ctx, lineID)
	if err != nil {
		return PurchaseLine{}, err
	}
	return ClonePurchaseLine(line), nil
}

func validDecision(decision MappingDecisionMetadata) error {
	if strings.TrimSpace(decision.Actor) == "" || decision.Origin != MappingOriginManual || decision.At.IsZero() {
		return NewError(InvalidArgument, "complete manual decision metadata is required")
	}
	return nil
}
