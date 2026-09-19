package resourcecore

import (
	"context"
	"strconv"
	"strings"
)

// WriteCapabilities is the Core-owned, service-shaped write seam. Only
// graduated operations appear here; a later per-operation change adds one
// method, which is an accepted per-slice seam change, not a consumer break.
type WriteCapabilities interface {
	CreateCatalog(context.Context, CatalogWriteRequest) (CatalogRecord, error)
	CreateResource(context.Context, ResourceWriteRequest) (Resource, error)
	UpdateCatalog(context.Context, CatalogUpdateRequest) (CatalogRecord, error)
	UpdateResource(context.Context, ResourceUpdateRequest) (Resource, error)
	DeactivateCatalog(context.Context, CatalogLifecycleRequest) (CatalogRecord, error)
	ReactivateCatalog(context.Context, CatalogLifecycleRequest) (CatalogRecord, error)
	DeactivateResource(context.Context, ResourceLifecycleRequest) (Resource, error)
	ReactivateResource(context.Context, ResourceLifecycleRequest) (Resource, error)
	HardDeleteCatalog(context.Context, CatalogLifecycleRequest) error
	UpdateAttributeOrder(context.Context, AttributeOrderWriteRequest) (ResourceAttributeOrder, error)
}

// Writer is the public write-only Resource Master contract. It validates
// request shape, delegates writes to WriteCapabilities, and defensively
// copies mutable collections crossing the boundary in both directions.
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

// CreateCatalog creates one catalog record of the requested kind.
func (w *Writer) CreateCatalog(ctx context.Context, req CatalogWriteRequest) (CatalogRecord, error) {
	if err := validateCatalogWriteRequest(req); err != nil {
		return CatalogRecord{}, err
	}
	rec, err := w.cap.CreateCatalog(ctx, CloneCatalogWriteRequest(req))
	if err != nil {
		return CatalogRecord{}, err
	}
	return CloneCatalogRecord(rec), nil
}

// CreateResource creates one resource.
func (w *Writer) CreateResource(ctx context.Context, req ResourceWriteRequest) (Resource, error) {
	if err := validateResourceWriteRequest(req); err != nil {
		return Resource{}, err
	}
	res, err := w.cap.CreateResource(ctx, CloneResourceWriteRequest(req))
	if err != nil {
		return Resource{}, err
	}
	return CloneResource(res), nil
}

// UpdateCatalog replaces one existing catalog record under optimistic
// concurrency.
func (w *Writer) UpdateCatalog(ctx context.Context, req CatalogUpdateRequest) (CatalogRecord, error) {
	if err := validateCatalogUpdateRequest(req); err != nil {
		return CatalogRecord{}, err
	}
	rec, err := w.cap.UpdateCatalog(ctx, CloneCatalogUpdateRequest(req))
	if err != nil {
		return CatalogRecord{}, err
	}
	return CloneCatalogRecord(rec), nil
}

// UpdateResource replaces one existing resource under optimistic
// concurrency.
func (w *Writer) UpdateResource(ctx context.Context, req ResourceUpdateRequest) (Resource, error) {
	if err := validateResourceUpdateRequest(req); err != nil {
		return Resource{}, err
	}
	res, err := w.cap.UpdateResource(ctx, CloneResourceUpdateRequest(req))
	if err != nil {
		return Resource{}, err
	}
	return CloneResource(res), nil
}

// DeactivateCatalog deactivates one existing catalog record under
// optimistic concurrency.
func (w *Writer) DeactivateCatalog(ctx context.Context, req CatalogLifecycleRequest) (CatalogRecord, error) {
	if err := validateCatalogLifecycleRequest(req); err != nil {
		return CatalogRecord{}, err
	}
	rec, err := w.cap.DeactivateCatalog(ctx, req)
	if err != nil {
		return CatalogRecord{}, err
	}
	return CloneCatalogRecord(rec), nil
}

// ReactivateCatalog reactivates one existing catalog record under
// optimistic concurrency.
func (w *Writer) ReactivateCatalog(ctx context.Context, req CatalogLifecycleRequest) (CatalogRecord, error) {
	if err := validateCatalogLifecycleRequest(req); err != nil {
		return CatalogRecord{}, err
	}
	rec, err := w.cap.ReactivateCatalog(ctx, req)
	if err != nil {
		return CatalogRecord{}, err
	}
	return CloneCatalogRecord(rec), nil
}

// DeactivateResource deactivates one existing resource under optimistic
// concurrency.
func (w *Writer) DeactivateResource(ctx context.Context, req ResourceLifecycleRequest) (Resource, error) {
	if err := validateResourceLifecycleRequest(req); err != nil {
		return Resource{}, err
	}
	res, err := w.cap.DeactivateResource(ctx, req)
	if err != nil {
		return Resource{}, err
	}
	return CloneResource(res), nil
}

// ReactivateResource reactivates one existing resource under optimistic
// concurrency.
func (w *Writer) ReactivateResource(ctx context.Context, req ResourceLifecycleRequest) (Resource, error) {
	if err := validateResourceLifecycleRequest(req); err != nil {
		return Resource{}, err
	}
	res, err := w.cap.ReactivateResource(ctx, req)
	if err != nil {
		return Resource{}, err
	}
	return CloneResource(res), nil
}

// HardDeleteCatalog permanently removes one existing catalog record under
// optimistic concurrency. Unlike Deactivate/Reactivate, there is no
// idempotent no-op target: a stale ExpectedRevision always yields CONFLICT.
// The internal capability returns no record — nothing is left to read back
// after a delete — so this method returns only an error.
func (w *Writer) HardDeleteCatalog(ctx context.Context, req CatalogLifecycleRequest) error {
	if err := validateCatalogLifecycleRequest(req); err != nil {
		return err
	}
	return w.cap.HardDeleteCatalog(ctx, req)
}

// UpdateAttributeOrder replaces one existing resource type's effective
// attribute presentation order under optimistic concurrency.
func (w *Writer) UpdateAttributeOrder(ctx context.Context, req AttributeOrderWriteRequest) (ResourceAttributeOrder, error) {
	if err := validateAttributeOrderWriteRequest(req); err != nil {
		return ResourceAttributeOrder{}, err
	}
	order, err := w.cap.UpdateAttributeOrder(ctx, CloneAttributeOrderWriteRequest(req))
	if err != nil {
		return ResourceAttributeOrder{}, err
	}
	return CloneResourceAttributeOrder(order), nil
}

func validateCatalogLifecycleRequest(req CatalogLifecycleRequest) error {
	if strings.TrimSpace(req.Actor) == "" {
		return NewError(InvalidArgument, "actor is required")
	}
	if req.ID == 0 {
		return NewError(InvalidArgument, "id is required")
	}
	if req.ExpectedRevision == 0 {
		return NewError(InvalidArgument, "expected revision is required")
	}
	return nil
}

func validateResourceLifecycleRequest(req ResourceLifecycleRequest) error {
	if strings.TrimSpace(req.Actor) == "" {
		return NewError(InvalidArgument, "actor is required")
	}
	if req.ID == 0 {
		return NewError(InvalidArgument, "id is required")
	}
	if req.ExpectedRevision == 0 {
		return NewError(InvalidArgument, "expected revision is required")
	}
	return nil
}

func validateCatalogWriteRequest(req CatalogWriteRequest) error {
	if strings.TrimSpace(req.Actor) == "" {
		return NewError(InvalidArgument, "actor is required")
	}
	if !isKnownKind(req.Kind) {
		return NewError(InvalidArgument, "invalid catalog kind")
	}
	if len(req.Values) == 0 {
		return NewError(InvalidArgument, "values are required")
	}
	for _, v := range req.Values {
		if err := validateValueShape(v); err != nil {
			return err
		}
	}
	for _, rule := range req.Rules {
		if rule.Equals.Kind != ValueText {
			return NewError(InvalidArgument, "applicability rule equals must be a TEXT value")
		}
		if !isKnownAttributeMode(rule.Mode) {
			return NewError(InvalidArgument, "invalid applicability rule mode")
		}
	}
	return nil
}

func validateResourceWriteRequest(req ResourceWriteRequest) error {
	if strings.TrimSpace(req.Actor) == "" {
		return NewError(InvalidArgument, "actor is required")
	}
	if strings.TrimSpace(req.Scope.ClassCode) == "" {
		return NewError(InvalidArgument, "class code is required")
	}
	if strings.TrimSpace(req.Scope.FamilyCode) == "" {
		return NewError(InvalidArgument, "family code is required")
	}
	if strings.TrimSpace(req.Scope.TypeCode) == "" {
		return NewError(InvalidArgument, "type code is required")
	}
	if strings.TrimSpace(req.NaturalUnit) == "" {
		return NewError(InvalidArgument, "natural unit is required")
	}
	for _, av := range req.Attributes {
		if err := validateValueShape(av.Value); err != nil {
			return err
		}
	}
	return nil
}

func validateCatalogUpdateRequest(req CatalogUpdateRequest) error {
	if strings.TrimSpace(req.Actor) == "" {
		return NewError(InvalidArgument, "actor is required")
	}
	if req.ID == 0 {
		return NewError(InvalidArgument, "id is required")
	}
	if req.ExpectedRevision == 0 {
		return NewError(InvalidArgument, "expected revision is required")
	}
	if !isKnownKind(req.Kind) {
		return NewError(InvalidArgument, "invalid catalog kind")
	}
	if len(req.Values) == 0 {
		return NewError(InvalidArgument, "values are required")
	}
	for _, v := range req.Values {
		if err := validateValueShape(v); err != nil {
			return err
		}
	}
	for _, rule := range req.Rules {
		if rule.Equals.Kind != ValueText {
			return NewError(InvalidArgument, "applicability rule equals must be a TEXT value")
		}
		if !isKnownAttributeMode(rule.Mode) {
			return NewError(InvalidArgument, "invalid applicability rule mode")
		}
	}
	return nil
}

func validateResourceUpdateRequest(req ResourceUpdateRequest) error {
	if strings.TrimSpace(req.Actor) == "" {
		return NewError(InvalidArgument, "actor is required")
	}
	if req.ID == 0 {
		return NewError(InvalidArgument, "id is required")
	}
	if req.ExpectedRevision == 0 {
		return NewError(InvalidArgument, "expected revision is required")
	}
	if strings.TrimSpace(req.Scope.ClassCode) == "" {
		return NewError(InvalidArgument, "class code is required")
	}
	if strings.TrimSpace(req.Scope.FamilyCode) == "" {
		return NewError(InvalidArgument, "family code is required")
	}
	if strings.TrimSpace(req.Scope.TypeCode) == "" {
		return NewError(InvalidArgument, "type code is required")
	}
	if strings.TrimSpace(req.NaturalUnit) == "" {
		return NewError(InvalidArgument, "natural unit is required")
	}
	for _, av := range req.Attributes {
		if err := validateValueShape(av.Value); err != nil {
			return err
		}
	}
	return nil
}

func validateValueShape(v Value) error {
	switch v.Kind {
	case ValueText, ValueCode, ValueBool, ValueInteger, ValueDecimal, ValueQuantity, ValueReference, ValueEnum, ValueStringList, ValueControlledOption, ValueNotApplicable:
	default:
		return NewError(InvalidArgument, "unknown value kind")
	}
	if v.Reference != nil && v.Reference.ID != 0 {
		return NewError(InvalidArgument, "reference id must be zero; references are by natural code")
	}
	if v.Kind == ValueQuantity {
		if strings.TrimSpace(v.UnitCode) == "" {
			return NewError(InvalidArgument, "unit code is required for quantity values")
		}
	} else if strings.TrimSpace(v.UnitCode) != "" {
		return NewError(InvalidArgument, "unit code is only valid for quantity values")
	}
	if v.Kind == ValueInteger {
		if _, err := strconv.ParseInt(v.Text, 10, 64); err != nil {
			return NewError(InvalidArgument, "invalid integer value")
		}
	}
	if v.Kind == ValueDecimal || v.Kind == ValueQuantity {
		if _, err := CanonicalDecimalString(v.Text); err != nil {
			return NewError(InvalidArgument, "invalid decimal value")
		}
	}
	return nil
}

func validateAttributeOrderWriteRequest(req AttributeOrderWriteRequest) error {
	if strings.TrimSpace(req.Actor) == "" {
		return NewError(InvalidArgument, "actor is required")
	}
	if strings.TrimSpace(req.Scope.ClassCode) == "" {
		return NewError(InvalidArgument, "class code is required")
	}
	if strings.TrimSpace(req.Scope.FamilyCode) == "" {
		return NewError(InvalidArgument, "family code is required")
	}
	if strings.TrimSpace(req.Scope.TypeCode) == "" {
		return NewError(InvalidArgument, "type code is required")
	}
	if strings.TrimSpace(req.ExpectedOrderRevision) == "" {
		return NewError(InvalidArgument, "expected order revision is required")
	}
	if len(req.OrderedAttributes) == 0 {
		return NewError(InvalidArgument, "ordered attributes are required")
	}
	return nil
}

func isKnownAttributeMode(mode string) bool {
	switch mode {
	case "REQUIRED", "OPTIONAL", "CONDITIONAL", "FORBIDDEN":
		return true
	}
	return false
}
