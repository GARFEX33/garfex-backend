package resourcecore

import (
	"context"
	"strings"
)

// CatalogKey identifies one catalog record for read.
type CatalogKey struct {
	Kind KindCode
	ID   int64
}

// ResourceKey identifies one resource for read or canonical description.
type ResourceKey struct {
	ClassCode  string
	IdentityV1 string
}

// ReadCapabilities is the service-shaped seam a Reader delegates to.
// Implementations are expected to be authoritative for the results they
// return and to translate their own internal errors to public Error values.
type ReadCapabilities interface {
	ActiveClasses(context.Context) ([]CatalogRecord, error)
	CatalogDescriptors(context.Context) ([]CatalogDescriptor, error)
	ListCatalog(context.Context, CatalogQuery) (CatalogPage, error)
	GetCatalog(context.Context, CatalogKey) (CatalogRecord, error)
	SearchResources(context.Context, ResourceQuery) (ResourcePage, error)
	GetResource(context.Context, ResourceKey) (Resource, error)
	DescribeResource(context.Context, ResourceKey) (string, error)
	EffectiveAttributesFor(context.Context, ResourceScope) ([]EffectiveAttribute, error)
	EvaluateAttributes(context.Context, ResourceScope, []AttributeValue) ([]EffectiveAttribute, error)
}

// Reader is the public read-only Resource Master contract. It validates
// request shape, delegates reads to ReadCapabilities, and defensively copies
// all returned mutable collections before exposing them.
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

// ActiveClasses returns active CLASE records.
func (r *Reader) ActiveClasses(ctx context.Context) ([]CatalogRecord, error) {
	recs, err := r.cap.ActiveClasses(ctx)
	if err != nil {
		return nil, err
	}
	return cloneCatalogRecordSlice(recs), nil
}

// CatalogDescriptors returns descriptors for every registered catalog kind.
func (r *Reader) CatalogDescriptors(ctx context.Context) ([]CatalogDescriptor, error) {
	descs, err := r.cap.CatalogDescriptors(ctx)
	if err != nil {
		return nil, err
	}
	return cloneCatalogDescriptorSlice(descs), nil
}

// ListCatalog returns a page of catalog records for the requested kind.
func (r *Reader) ListCatalog(ctx context.Context, q CatalogQuery) (CatalogPage, error) {
	if err := validateCatalogQuery(q); err != nil {
		return CatalogPage{}, err
	}
	page, err := r.cap.ListCatalog(ctx, q)
	if err != nil {
		return CatalogPage{}, err
	}
	page.Records = cloneCatalogRecordSlice(page.Records)
	return page, nil
}

// GetCatalog returns one catalog record by kind and id.
func (r *Reader) GetCatalog(ctx context.Context, key CatalogKey) (CatalogRecord, error) {
	if err := validateCatalogKey(key); err != nil {
		return CatalogRecord{}, err
	}
	rec, err := r.cap.GetCatalog(ctx, key)
	if err != nil {
		return CatalogRecord{}, err
	}
	return CloneCatalogRecord(rec), nil
}

// SearchResources returns a page of resources matching the query.
func (r *Reader) SearchResources(ctx context.Context, q ResourceQuery) (ResourcePage, error) {
	if err := validateResourceQuery(q); err != nil {
		return ResourcePage{}, err
	}
	page, err := r.cap.SearchResources(ctx, q)
	if err != nil {
		return ResourcePage{}, err
	}
	page.Resources = cloneResourceSlice(page.Resources)
	return page, nil
}

// GetResource returns one resource by class code and durable identity-v1.
func (r *Reader) GetResource(ctx context.Context, key ResourceKey) (Resource, error) {
	if err := validateResourceKey(key); err != nil {
		return Resource{}, err
	}
	res, err := r.cap.GetResource(ctx, key)
	if err != nil {
		return Resource{}, err
	}
	return CloneResource(res), nil
}

// DescribeResource returns the canonical presentation of the identified
// resource. The capability is responsible for delegating to the authoritative
// resource service.
func (r *Reader) DescribeResource(ctx context.Context, key ResourceKey) (string, error) {
	if err := validateResourceKey(key); err != nil {
		return "", err
	}
	return r.cap.DescribeResource(ctx, key)
}

// EffectiveAttributesFor returns the resolved attribute view for one
// ResourceType: inheritance, presentation order, and rule evaluation already
// applied (see domain.ResourceCatalog.EffectiveAttributesFor) — a caller
// never has to re-implement herencia or Rules evaluation itself.
func (r *Reader) EffectiveAttributesFor(ctx context.Context, scope ResourceScope) ([]EffectiveAttribute, error) {
	if err := validateEffectiveAttributesScope(scope); err != nil {
		return nil, err
	}
	attrs, err := r.cap.EffectiveAttributesFor(ctx, scope)
	if err != nil {
		return nil, err
	}
	return cloneEffectiveAttributeSlice(attrs), nil
}

// EvaluateAttributes resolves scope's applicable attributes against values —
// the same resolution as EffectiveAttributesFor, but with the caller's
// already-known values folded into CONDITIONAL rule evaluation and relation-
// narrowed Options (see domain.ResourceCatalog.EffectiveAttributesFor).
func (r *Reader) EvaluateAttributes(ctx context.Context, scope ResourceScope, values []AttributeValue) ([]EffectiveAttribute, error) {
	if err := validateEffectiveAttributesScope(scope); err != nil {
		return nil, err
	}
	attrs, err := r.cap.EvaluateAttributes(ctx, scope, cloneAttributeValueSlice(values))
	if err != nil {
		return nil, err
	}
	return cloneEffectiveAttributeSlice(attrs), nil
}

func validateCatalogQuery(q CatalogQuery) error {
	if !isKnownKind(q.Kind) {
		return NewError(InvalidArgument, "invalid catalog kind")
	}
	if q.Scope != "" && q.Scope != ScopeActive && q.Scope != ScopeInactive && q.Scope != ScopeAll {
		return NewError(InvalidArgument, "invalid lifecycle scope")
	}
	return nil
}

func validateCatalogKey(key CatalogKey) error {
	if !isKnownKind(key.Kind) {
		return NewError(InvalidArgument, "invalid catalog kind")
	}
	if key.ID <= 0 {
		return NewError(InvalidArgument, "catalog id must be positive")
	}
	return nil
}

func validateResourceQuery(q ResourceQuery) error {
	if q.Scope != "" && q.Scope != ScopeActive && q.Scope != ScopeInactive && q.Scope != ScopeAll {
		return NewError(InvalidArgument, "invalid lifecycle scope")
	}
	return nil
}

func validateResourceKey(key ResourceKey) error {
	if strings.TrimSpace(key.ClassCode) == "" {
		return NewError(InvalidArgument, "class code is required")
	}
	if strings.TrimSpace(key.IdentityV1) == "" {
		return NewError(InvalidArgument, "identity is required")
	}
	return nil
}

func validateEffectiveAttributesScope(scope ResourceScope) error {
	if strings.TrimSpace(scope.ClassCode) == "" {
		return NewError(InvalidArgument, "class code is required")
	}
	if strings.TrimSpace(scope.FamilyCode) == "" {
		return NewError(InvalidArgument, "family code is required")
	}
	if strings.TrimSpace(scope.TypeCode) == "" {
		return NewError(InvalidArgument, "type code is required")
	}
	return nil
}

func isKnownKind(k KindCode) bool {
	switch k {
	case KindClass, KindFamily, KindType, KindAttributeDefinition, KindOptionSet,
		KindOption, KindOptionRelation, KindUnit, KindUnitPolicy,
		KindAttributeBinding, KindPresentationField:
		return true
	}
	return false
}

func cloneCatalogRecordSlice(recs []CatalogRecord) []CatalogRecord {
	if recs == nil {
		return nil
	}
	out := make([]CatalogRecord, len(recs))
	for i := range recs {
		out[i] = CloneCatalogRecord(recs[i])
	}
	return out
}

func cloneAttributeValueSlice(values []AttributeValue) []AttributeValue {
	if values == nil {
		return nil
	}
	out := make([]AttributeValue, len(values))
	for i, v := range values {
		out[i] = AttributeValue{Code: v.Code, Value: CloneValue(v.Value), UnitCode: v.UnitCode}
	}
	return out
}

func cloneEffectiveAttributeSlice(attrs []EffectiveAttribute) []EffectiveAttribute {
	if attrs == nil {
		return nil
	}
	out := make([]EffectiveAttribute, len(attrs))
	for i := range attrs {
		out[i] = CloneEffectiveAttribute(attrs[i])
	}
	return out
}

func cloneCatalogDescriptorSlice(descs []CatalogDescriptor) []CatalogDescriptor {
	if descs == nil {
		return nil
	}
	out := make([]CatalogDescriptor, len(descs))
	for i := range descs {
		out[i] = CloneCatalogDescriptor(descs[i])
	}
	return out
}

func cloneResourceSlice(resources []Resource) []Resource {
	if resources == nil {
		return nil
	}
	out := make([]Resource, len(resources))
	for i := range resources {
		out[i] = CloneResource(resources[i])
	}
	return out
}
