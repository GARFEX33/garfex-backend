// Package resourcecore is the internal bridge between the public resourcecore
// read contract and the authoritative application services. It translates
// public requests to domain calls, maps neutral errors to public errors, and
// defensively copies data crossing the boundary.
package resourcecore

import (
	"context"
	"fmt"
	"strconv"

	"github.com/shopspring/decimal"

	"github.com/GARFEX33/garfex-backend/internal/core"
	"github.com/GARFEX33/garfex-backend/internal/domain"
	public "github.com/GARFEX33/garfex-backend/resourcecore"
)

// catalogReader is the narrow catalog read seam the bridge consumes.
type catalogReader interface {
	Kinds() []domain.CatalogKind
	List(ctx context.Context, kind domain.CatalogKindCode, filter domain.CatalogFilter) ([]domain.CatalogRecord, error)
	Get(ctx context.Context, kind domain.CatalogKindCode, id int64) (domain.CatalogRecord, error)
}

// catalogWriter is the narrow catalog write seam the bridge consumes. It
// binds to catalogo.Service.Create, never CreateV2: composition, not
// translation, decides which repository backs the service.
type catalogWriter interface {
	Create(ctx context.Context, kind domain.CatalogKindCode, rec domain.CatalogRecord) (domain.CatalogRecord, error)
	UpdateRevision(ctx context.Context, kind domain.CatalogKindCode, rec domain.CatalogRecord, expectedRevision uint64) (domain.CatalogRecord, error)
	DeactivateRevision(ctx context.Context, kind domain.CatalogKindCode, id int64, expectedRevision uint64) error
	ReactivateRevision(ctx context.Context, kind domain.CatalogKindCode, id int64, expectedRevision uint64) error
	HardDeleteRevision(ctx context.Context, kind domain.CatalogKindCode, id int64, expectedRevision uint64) error
}

// catalogPort is the complete catalog seam: reads plus the one graduated
// write operation.
type catalogPort interface {
	catalogReader
	catalogWriter
}

// resourceReader is the narrow resource read seam the bridge consumes.
type resourceReader interface {
	Get(ctx context.Context, classCode, identityKey string) (domain.Resource, error)
	SearchPage(ctx context.Context, criteria domain.SearchCriteria) (domain.ResourcePage, error)
	Describe(resource domain.Resource) string
	EffectiveAttributes(scope domain.ResourceScope, current []domain.ResourceAttributeValue) ([]domain.EffectiveAttribute, error)
	ReadAttributeOrder(ctx context.Context, scope domain.ResourceScope) (domain.AttributeOrderReadResult, error)
}

// resourceWriter is the narrow resource write seam the bridge consumes.
type resourceWriter interface {
	Create(ctx context.Context, command domain.CreateCommand) (domain.Resource, error)
	UpdateRevision(ctx context.Context, command domain.UpdateCommand, expectedRevision uint64) (domain.Resource, error)
	DeactivateRevision(ctx context.Context, id int64, expectedRevision uint64) (domain.LifecycleResult, error)
	ReactivateRevision(ctx context.Context, id int64, expectedRevision uint64) (domain.LifecycleResult, error)
	WriteAttributeOrder(ctx context.Context, req domain.AttributeOrderWriteRequest) (domain.AttributeOrderReadResult, error)
}

// resourcePort is the complete resource seam: reads plus the one graduated
// write operation.
type resourcePort interface {
	resourceReader
	resourceWriter
}

// Adapter implements public.ReadCapabilities and public.WriteCapabilities by
// delegating to the internal catalog and resource application services.
type Adapter struct {
	catalog   catalogPort
	resources resourcePort
	kinds     map[domain.CatalogKindCode]domain.CatalogKind
}

var (
	_ public.ReadCapabilities  = (*Adapter)(nil)
	_ public.WriteCapabilities = (*Adapter)(nil)
)

// NewAdapter returns a ReadCapabilities/WriteCapabilities implementation
// backed by catalog and resources. Neither dependency may be nil.
func NewAdapter(catalog catalogPort, resources resourcePort) *Adapter {
	kinds := make(map[domain.CatalogKindCode]domain.CatalogKind, len(catalog.Kinds()))
	for _, kind := range catalog.Kinds() {
		kinds[kind.Code] = kind
	}
	return &Adapter{catalog: catalog, resources: resources, kinds: kinds}
}

func (a *Adapter) fieldKind(kind domain.CatalogKindCode, name string) domain.FieldKind {
	k, ok := a.kinds[kind]
	if !ok {
		return domain.FieldText
	}
	for _, f := range k.Fields {
		if f.Name == name {
			return f.Kind
		}
	}
	return domain.FieldText
}

// ActiveClasses returns active CLASE records.
func (a *Adapter) ActiveClasses(ctx context.Context) ([]public.CatalogRecord, error) {
	recs, err := a.catalog.List(ctx, domain.KindClass, domain.CatalogFilter{Status: domain.CatalogStatusActive})
	if err != nil {
		return nil, mapError(err)
	}
	return a.mapCatalogRecordSlice(domain.KindClass, recs), nil
}

// CatalogDescriptors returns a public descriptor for every registered kind.
func (a *Adapter) CatalogDescriptors(ctx context.Context) ([]public.CatalogDescriptor, error) {
	_ = ctx
	descs := make([]public.CatalogDescriptor, 0, len(a.catalog.Kinds()))
	for _, kind := range a.catalog.Kinds() {
		descs = append(descs, mapCatalogDescriptor(kind))
	}
	return descs, nil
}

// ListCatalog returns a page of catalog records for the requested kind.
func (a *Adapter) ListCatalog(ctx context.Context, q public.CatalogQuery) (public.CatalogPage, error) {
	filter := domain.CatalogFilter{
		Status: catalogStatusFromPublic(q.Scope),
		Text:   q.Text,
		Parent: catalogParentFilter(q),
		Limit:  q.Limit + 1,
		Offset: q.Offset,
	}
	recs, err := a.catalog.List(ctx, domain.CatalogKindCode(q.Kind), filter)
	if err != nil {
		return public.CatalogPage{}, mapError(err)
	}
	return a.buildCatalogPage(q, recs), nil
}

// catalogParentFilter builds the internal Parent scoping map from the
// public query's ClassCode/FamilyCode/TypeCode/OptionSetCode/
// CharacteristicCode. A kind whose descriptor does not name that parent
// field silently ignores the corresponding key (see parentRefCode in
// internal/postgres) — never an error.
func catalogParentFilter(q public.CatalogQuery) map[string]domain.CatalogValue {
	if q.ClassCode == "" && q.FamilyCode == "" && q.TypeCode == "" && q.OptionSetCode == "" && q.CharacteristicCode == "" {
		return nil
	}
	parent := map[string]domain.CatalogValue{}
	if q.ClassCode != "" {
		parent["class"] = domain.CatalogValue{Ref: domain.CatalogRef{Kind: domain.KindClass, Code: q.ClassCode}}
	}
	if q.FamilyCode != "" {
		parent["family"] = domain.CatalogValue{Ref: domain.CatalogRef{Kind: domain.KindFamily, Code: q.FamilyCode}}
	}
	if q.TypeCode != "" {
		parent["type"] = domain.CatalogValue{Ref: domain.CatalogRef{Kind: domain.KindType, Code: q.TypeCode}}
	}
	if q.OptionSetCode != "" {
		parent["optionSet"] = domain.CatalogValue{Ref: domain.CatalogRef{Kind: domain.KindOptionSet, Code: q.OptionSetCode}}
	}
	if q.CharacteristicCode != "" {
		parent["characteristic"] = domain.CatalogValue{Ref: domain.CatalogRef{Kind: domain.KindAttributeDefinition, Code: q.CharacteristicCode}}
	}
	return parent
}

// GetCatalog returns one catalog record by kind and id.
func (a *Adapter) GetCatalog(ctx context.Context, key public.CatalogKey) (public.CatalogRecord, error) {
	rec, err := a.catalog.Get(ctx, domain.CatalogKindCode(key.Kind), key.ID)
	if err != nil {
		return public.CatalogRecord{}, mapError(err)
	}
	return a.mapCatalogRecord(domain.CatalogKindCode(key.Kind), rec), nil
}

// CreateCatalog creates one catalog record of the requested kind. Actor
// travels only as request-scoped context metadata for diagnostic
// attribution; it is never a business parameter and is never persisted. A
// confirm-read follows the write (mirroring CreateResource/Deactivate/
// ReactivateCatalog) because the write itself only ever echoes back the
// caller-submitted Values — a reference field there carries no real ID
// (the client only ever sends Code), so a fresh Get resolves the persisted
// row's actual reference IDs instead of the client's implicit zero.
func (a *Adapter) CreateCatalog(ctx context.Context, req public.CatalogWriteRequest) (public.CatalogRecord, error) {
	kind := domain.CatalogKindCode(req.Kind)
	rec := domain.CatalogRecord{
		Active: req.Active,
		Values: a.toDomainCatalogValues(kind, req.Values),
		Rules:  toDomainCatalogRules(req.Rules),
	}
	created, err := a.catalog.Create(core.WithActor(ctx, req.Actor), kind, rec)
	if err != nil {
		return public.CatalogRecord{}, mapError(err)
	}
	confirmed, err := a.catalog.Get(ctx, kind, created.ID)
	if err != nil {
		return public.CatalogRecord{}, mapError(err)
	}
	return a.mapCatalogRecord(kind, confirmed), nil
}

// UpdateCatalog replaces one existing catalog record under optimistic
// concurrency. Reuses the exact inverse value/rule mappers Create built; no
// new mapping code. Actor travels only as request-scoped context metadata,
// exactly as in CreateCatalog. A confirm-read follows the write for the
// same reason as CreateCatalog's own (see its doc comment): the write
// result otherwise echoes the client's zero-ID reference values.
func (a *Adapter) UpdateCatalog(ctx context.Context, req public.CatalogUpdateRequest) (public.CatalogRecord, error) {
	kind := domain.CatalogKindCode(req.Kind)
	rec := domain.CatalogRecord{
		ID:     req.ID,
		Active: req.Active,
		Values: a.toDomainCatalogValues(kind, req.Values),
		Rules:  toDomainCatalogRules(req.Rules),
	}
	if _, err := a.catalog.UpdateRevision(core.WithActor(ctx, req.Actor), kind, rec, req.ExpectedRevision); err != nil {
		return public.CatalogRecord{}, mapError(err)
	}
	confirmed, err := a.catalog.Get(ctx, kind, req.ID)
	if err != nil {
		return public.CatalogRecord{}, mapError(err)
	}
	return a.mapCatalogRecord(kind, confirmed), nil
}

// UpdateResource replaces one existing resource under optimistic
// concurrency. No create-confirm read: ResourceRepositoryV2.UpdateRevision
// returns the real persisted Resource directly.
func (a *Adapter) UpdateResource(ctx context.Context, req public.ResourceUpdateRequest) (public.Resource, error) {
	attrs, err := toDomainResourceAttributes(req.Attributes)
	if err != nil {
		return public.Resource{}, err
	}
	command := domain.UpdateCommand{
		ID:          req.ID,
		Scope:       domain.ResourceScope{ClassCode: req.Scope.ClassCode, FamilyCode: req.Scope.FamilyCode, TypeCode: req.Scope.TypeCode},
		NaturalUnit: req.NaturalUnit,
		Attributes:  attrs,
	}
	updated, err := a.resources.UpdateRevision(core.WithActor(ctx, req.Actor), command, req.ExpectedRevision)
	if err != nil {
		return public.Resource{}, mapError(err)
	}
	return mapResource(updated), nil
}

// DeactivateCatalog deactivates one existing catalog record under
// optimistic concurrency. DeactivateRevision returns only error — no
// record, because postgres's SetActive builds a deliberately partial
// 4-field domain.CatalogRecord projection even at the domain-port level. A
// confirm-read recovers the complete public record, the same gap-recovery
// shape CreateResource already established for a different underlying gap.
// A confirm-read failure is not given a distinct error category: it flows
// through the same mapError any other Get failure would use — never told
// the toggle itself failed when the persisted state has, in fact, already
// changed.
func (a *Adapter) DeactivateCatalog(ctx context.Context, req public.CatalogLifecycleRequest) (public.CatalogRecord, error) {
	kind := domain.CatalogKindCode(req.Kind)
	if err := a.catalog.DeactivateRevision(core.WithActor(ctx, req.Actor), kind, req.ID, req.ExpectedRevision); err != nil {
		return public.CatalogRecord{}, mapError(err)
	}
	rec, err := a.catalog.Get(ctx, kind, req.ID)
	if err != nil {
		return public.CatalogRecord{}, mapError(err)
	}
	return a.mapCatalogRecord(kind, rec), nil
}

// ReactivateCatalog reactivates one existing catalog record under
// optimistic concurrency. Same confirm-read shape as DeactivateCatalog.
func (a *Adapter) ReactivateCatalog(ctx context.Context, req public.CatalogLifecycleRequest) (public.CatalogRecord, error) {
	kind := domain.CatalogKindCode(req.Kind)
	if err := a.catalog.ReactivateRevision(core.WithActor(ctx, req.Actor), kind, req.ID, req.ExpectedRevision); err != nil {
		return public.CatalogRecord{}, mapError(err)
	}
	rec, err := a.catalog.Get(ctx, kind, req.ID)
	if err != nil {
		return public.CatalogRecord{}, mapError(err)
	}
	return a.mapCatalogRecord(kind, rec), nil
}

// DeactivateResource deactivates one existing resource under optimistic
// concurrency. No confirm-read: domain.LifecycleResult.Resource is already
// the complete, persisted resource.
func (a *Adapter) DeactivateResource(ctx context.Context, req public.ResourceLifecycleRequest) (public.Resource, error) {
	result, err := a.resources.DeactivateRevision(core.WithActor(ctx, req.Actor), req.ID, req.ExpectedRevision)
	if err != nil {
		return public.Resource{}, mapError(err)
	}
	return mapResource(result.Resource), nil
}

// ReactivateResource reactivates one existing resource under optimistic
// concurrency. Same no-confirm-read shape as DeactivateResource.
func (a *Adapter) ReactivateResource(ctx context.Context, req public.ResourceLifecycleRequest) (public.Resource, error) {
	result, err := a.resources.ReactivateRevision(core.WithActor(ctx, req.Actor), req.ID, req.ExpectedRevision)
	if err != nil {
		return public.Resource{}, mapError(err)
	}
	return mapResource(result.Resource), nil
}

// HardDeleteCatalog permanently removes one existing catalog record under
// optimistic concurrency. No confirm-read: HardDeleteRevision returns only
// error, and the deleted record no longer exists to read back. The bridge
// evaluates no guard of its own — active-target, dependents, and
// resource-reference rejection all belong to catalogo.Service.HardDeleteRevision
// alone; this method is a pure translate-and-delegate pass-through.
func (a *Adapter) HardDeleteCatalog(ctx context.Context, req public.CatalogLifecycleRequest) error {
	kind := domain.CatalogKindCode(req.Kind)
	return mapError(a.catalog.HardDeleteRevision(core.WithActor(ctx, req.Actor), kind, req.ID, req.ExpectedRevision))
}

// SearchResources returns a page of resources matching the query.
func (a *Adapter) SearchResources(ctx context.Context, q public.ResourceQuery) (public.ResourcePage, error) {
	if q.Scope == public.ScopeAll {
		return public.ResourcePage{}, public.NewError(public.InvalidArgument, "ScopeAll is not supported for resource search")
	}
	criteria := domain.SearchCriteria{
		LifecycleScope: resourceLifecycleFromPublic(q.Scope),
		ClassCode:      q.ClassCode,
		FamilyCode:     q.FamilyCode,
		TypeCode:       q.TypeCode,
		Text:           q.Text,
		Limit:          q.Limit,
		Offset:         q.Offset,
	}
	page, err := a.resources.SearchPage(ctx, criteria)
	if err != nil {
		return public.ResourcePage{}, mapError(err)
	}
	return public.ResourcePage{
		Query:       mapResourceQuery(page.Criteria),
		Resources:   mapResourceSlice(page.Resources),
		HasPrevious: page.HasPrevious,
		HasNext:     page.HasNext,
	}, nil
}

// GetResource returns one resource by class code and durable identity-v1.
func (a *Adapter) GetResource(ctx context.Context, key public.ResourceKey) (public.Resource, error) {
	res, err := a.resources.Get(ctx, key.ClassCode, key.IdentityV1)
	if err != nil {
		return public.Resource{}, mapError(err)
	}
	return mapResource(res), nil
}

// CreateResource creates one resource. Because internal/postgres's resource
// repository Create returns only an error (no persisted ID or revision),
// this performs one non-transactional create-confirm read by the resource's
// canonical identity-v1 to project the real persisted values; safe only
// under the approved one-writer topology. A confirm-read failure passes
// through the ordinary mapError unchanged — never reclassified.
func (a *Adapter) CreateResource(ctx context.Context, req public.ResourceWriteRequest) (public.Resource, error) {
	attrs, err := toDomainResourceAttributes(req.Attributes)
	if err != nil {
		return public.Resource{}, err
	}
	command := domain.CreateCommand{
		Scope:       domain.ResourceScope{ClassCode: req.Scope.ClassCode, FamilyCode: req.Scope.FamilyCode, TypeCode: req.Scope.TypeCode},
		NaturalUnit: req.NaturalUnit,
		Attributes:  attrs,
	}
	actorCtx := core.WithActor(ctx, req.Actor)
	created, err := a.resources.Create(actorCtx, command)
	if err != nil {
		return public.Resource{}, mapError(err)
	}
	confirmed, err := a.resources.Get(actorCtx, created.ClassCode, created.IdentityKey)
	if err != nil {
		return public.Resource{}, mapError(err)
	}
	return mapResource(confirmed), nil
}

// DescribeResource returns the canonical presentation of the identified
// resource by delegating to the resource service's Describe method.
func (a *Adapter) DescribeResource(ctx context.Context, key public.ResourceKey) (string, error) {
	res, err := a.resources.Get(ctx, key.ClassCode, key.IdentityV1)
	if err != nil {
		return "", mapError(err)
	}
	return a.resources.Describe(res), nil
}

// EffectiveAttributesFor returns the resolved attribute view for one
// ResourceType, delegating to the resource service's pure in-memory
// resolution (inheritance, presentation order, and rule evaluation already
// applied — see domain.ResourceCatalog.EffectiveAttributesFor). current is
// always nil from this read path; a future contextual-evaluation capability
// would pass real values through the same domain method.
func (a *Adapter) EffectiveAttributesFor(ctx context.Context, scope public.ResourceScope) ([]public.EffectiveAttribute, error) {
	_ = ctx
	domainScope := domain.ResourceScope{ClassCode: scope.ClassCode, FamilyCode: scope.FamilyCode, TypeCode: scope.TypeCode}
	attributes, err := a.resources.EffectiveAttributes(domainScope, nil)
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]public.EffectiveAttribute, len(attributes))
	for i, attribute := range attributes {
		out[i] = mapEffectiveAttribute(attribute)
	}
	return out, nil
}

// EvaluateAttributes resolves the same attribute view as
// EffectiveAttributesFor, but with values folded into CONDITIONAL rule
// evaluation and relation-narrowed Options — reusing the exact same
// resource-service resolution, only with real values instead of nil.
func (a *Adapter) EvaluateAttributes(ctx context.Context, scope public.ResourceScope, values []public.AttributeValue) ([]public.EffectiveAttribute, error) {
	_ = ctx
	domainValues, err := toDomainResourceAttributes(values)
	if err != nil {
		return nil, err
	}
	domainScope := domain.ResourceScope{ClassCode: scope.ClassCode, FamilyCode: scope.FamilyCode, TypeCode: scope.TypeCode}
	attributes, err := a.resources.EffectiveAttributes(domainScope, domainValues)
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]public.EffectiveAttribute, len(attributes))
	for i, attribute := range attributes {
		out[i] = mapEffectiveAttribute(attribute)
	}
	return out, nil
}

// AttributeOrderFor returns the per-Tipo visual order snapshot for one
// exact {classCode,familyCode,typeCode} scope. It maps only
// domain.AttributeOrderReadResult.Order into the public DTO; .Catalog is an
// internal structural-catalog snapshot used only to resolve the baseline
// inside the domain/postgres layers and is never exposed here (no database
// IDs, and no field of public.ResourceAttributeOrder could carry it).
func (a *Adapter) AttributeOrderFor(ctx context.Context, scope public.ResourceScope) (public.ResourceAttributeOrder, error) {
	domainScope := domain.ResourceScope{ClassCode: scope.ClassCode, FamilyCode: scope.FamilyCode, TypeCode: scope.TypeCode}
	result, err := a.resources.ReadAttributeOrder(ctx, domainScope)
	if err != nil {
		return public.ResourceAttributeOrder{}, mapError(err)
	}
	return mapAttributeOrderSnapshot(result.Order), nil
}

// UpdateAttributeOrder replaces one existing resource type's effective
// attribute presentation order under optimistic concurrency. req.Actor is
// audit metadata, not authentication, carried via core.WithActor exactly as
// UpdateResource already does. Like AttributeOrderFor, only
// domain.AttributeOrderReadResult.Order is mapped into the public DTO;
// .Catalog is never exposed.
func (a *Adapter) UpdateAttributeOrder(ctx context.Context, req public.AttributeOrderWriteRequest) (public.ResourceAttributeOrder, error) {
	domainScope := domain.ResourceScope{ClassCode: req.Scope.ClassCode, FamilyCode: req.Scope.FamilyCode, TypeCode: req.Scope.TypeCode}
	domainReq := domain.AttributeOrderWriteRequest{
		Scope:                 domainScope,
		ExpectedOrderRevision: req.ExpectedOrderRevision,
		OrderedAttributes:     toDomainAttributeOrderKeys(req.OrderedAttributes),
	}
	result, err := a.resources.WriteAttributeOrder(core.WithActor(ctx, req.Actor), domainReq)
	if err != nil {
		return public.ResourceAttributeOrder{}, mapError(err)
	}
	return mapAttributeOrderSnapshot(result.Order), nil
}

func mapAttributeOrderSnapshot(order domain.AttributeOrderSnapshot) public.ResourceAttributeOrder {
	return public.ResourceAttributeOrder{
		Scope:             public.ResourceScope{ClassCode: order.Scope.ClassCode, FamilyCode: order.Scope.FamilyCode, TypeCode: order.Scope.TypeCode},
		OrderedAttributes: mapAttributeOrderKeys(order.OrderedAttributes),
		OrderRevision:     order.OrderRevision,
	}
}

func mapAttributeOrderKeys(keys []domain.AttributeOrderKey) []public.AttributeOrderKey {
	out := make([]public.AttributeOrderKey, len(keys))
	for i, k := range keys {
		out[i] = public.AttributeOrderKey{SourceLevel: k.SourceLevel, SourceCode: k.SourceCode, CharacteristicCode: k.CharacteristicCode}
	}
	return out
}

func toDomainAttributeOrderKeys(keys []public.AttributeOrderKey) []domain.AttributeOrderKey {
	out := make([]domain.AttributeOrderKey, len(keys))
	for i, k := range keys {
		out[i] = domain.AttributeOrderKey{SourceLevel: k.SourceLevel, SourceCode: k.SourceCode, CharacteristicCode: k.CharacteristicCode}
	}
	return out
}

func mapEffectiveAttribute(a domain.EffectiveAttribute) public.EffectiveAttribute {
	return public.EffectiveAttribute{
		Characteristic: public.CharacteristicDescriptor{
			Code: a.Attribute.Definition.Code, Name: a.Attribute.Definition.Name,
			ValueType: string(a.Attribute.Definition.ValueType), Dimension: a.Attribute.Definition.Dimension,
		},
		EffectiveMode:        string(a.EffectiveMode),
		IdentityParticipates: a.IdentityParticipates,
		NotApplicable:        a.NotApplicable,
		Position:             a.Position,
		HasPosition:          a.HasPosition,
		OptionSetCode:        a.Attribute.OptionSet,
		Source:               public.EffectiveAttributeSource{Level: a.SourceLevel, Code: a.SourceCode},
		Rules:                mapAttributeRules(a.Attribute.Rules),
		Options:              mapAttributeOptions(a.Options),
	}
}

func mapAttributeOptions(options []domain.AttributeOption) []public.EffectiveAttributeOption {
	if options == nil {
		return nil
	}
	out := make([]public.EffectiveAttributeOption, len(options))
	for i, o := range options {
		out[i] = public.EffectiveAttributeOption{Code: o.Code, Label: o.Label}
	}
	return out
}

func mapAttributeRules(rules []domain.AttributeRule) []public.ApplicabilityRule {
	if rules == nil {
		return nil
	}
	out := make([]public.ApplicabilityRule, len(rules))
	for i, r := range rules {
		out[i] = public.ApplicabilityRule{
			AttributeCode:        r.When.AttributeCode,
			Equals:               public.Value{Kind: public.ValueText, Text: r.When.Equals},
			Mode:                 string(r.Mode),
			IdentityParticipates: r.IdentityParticipates,
			NotApplicable:        r.NotApplicable,
			Active:               r.Active,
		}
	}
	return out
}

func mapError(err error) error {
	if err == nil {
		return nil
	}
	neut := core.Map(err)
	return public.NewError(public.ErrorCode(neut.Code()), neut.Error())
}

func catalogStatusFromPublic(s public.LifecycleScope) domain.CatalogStatus {
	switch s {
	case public.ScopeInactive:
		return domain.CatalogStatusInactive
	case public.ScopeAll:
		return domain.CatalogStatusAll
	}
	return domain.CatalogStatusActive
}

func resourceLifecycleFromPublic(s public.LifecycleScope) domain.ResourceLifecycleScope {
	if s == public.ScopeInactive {
		return domain.LifecycleScopeInactive
	}
	return domain.LifecycleScopeActive
}

func (a *Adapter) buildCatalogPage(q public.CatalogQuery, recs []domain.CatalogRecord) public.CatalogPage {
	hasNext := len(recs) > q.Limit && q.Limit > 0
	end := len(recs)
	if hasNext {
		end = q.Limit
	}
	if q.Limit == 0 {
		hasNext = false
	}
	return public.CatalogPage{
		Query: public.CatalogQuery{
			Kind:               q.Kind,
			Scope:              q.Scope,
			Text:               q.Text,
			ClassCode:          q.ClassCode,
			FamilyCode:         q.FamilyCode,
			TypeCode:           q.TypeCode,
			OptionSetCode:      q.OptionSetCode,
			CharacteristicCode: q.CharacteristicCode,
			Limit:              q.Limit,
			Offset:             q.Offset,
		},
		Records:     a.mapCatalogRecordSlice(domain.CatalogKindCode(q.Kind), recs[:end]),
		HasPrevious: q.Offset > 0,
		HasNext:     hasNext,
	}
}

func mapCatalogDescriptor(kind domain.CatalogKind) public.CatalogDescriptor {
	fields := make([]public.FieldDescriptor, len(kind.Fields))
	for i, f := range kind.Fields {
		fields[i] = public.FieldDescriptor{
			Name:        f.Name,
			Label:       f.Label,
			Kind:        valueKindFromFieldKind(f.Kind),
			Required:    f.Required,
			RefKind:     public.KindCode(f.RefKind),
			RefScopedBy: append([]string(nil), f.RefScopedBy...),
			AllowCreate: f.AllowCreate,
			EnumValues:  mapEnumValues(f.EnumValues),
		}
	}
	identity := make([]string, len(kind.IdentityFields))
	copy(identity, kind.IdentityFields)
	return public.CatalogDescriptor{
		Kind:           public.KindCode(kind.Code),
		Singular:       kind.Singular,
		Plural:         kind.Plural,
		Fields:         fields,
		IdentityFields: identity,
		ParentKind:     public.KindCode(kind.ParentKind),
		ParentField:    kind.ParentField,
	}
}

func mapEnumValues(values []domain.EnumValue) []public.EnumValue {
	if values == nil {
		return nil
	}
	out := make([]public.EnumValue, len(values))
	for i, v := range values {
		out[i] = public.EnumValue{Value: v.Value, Label: v.Label}
	}
	return out
}

func valueKindFromFieldKind(k domain.FieldKind) public.ValueKind {
	switch k {
	case domain.FieldText:
		return public.ValueText
	case domain.FieldCode:
		return public.ValueCode
	case domain.FieldBool:
		return public.ValueBool
	case domain.FieldInt:
		return public.ValueInteger
	case domain.FieldRef:
		return public.ValueReference
	case domain.FieldEnum:
		return public.ValueEnum
	case domain.FieldStringList:
		return public.ValueStringList
	}
	return public.ValueText
}

func (a *Adapter) mapCatalogRecordSlice(kind domain.CatalogKindCode, recs []domain.CatalogRecord) []public.CatalogRecord {
	out := make([]public.CatalogRecord, len(recs))
	for i, rec := range recs {
		out[i] = a.mapCatalogRecord(kind, rec)
	}
	return out
}

func (a *Adapter) mapCatalogRecord(kind domain.CatalogKindCode, rec domain.CatalogRecord) public.CatalogRecord {
	values := make(map[string]public.Value, len(rec.Values))
	for name, value := range rec.Values {
		values[name] = a.mapCatalogValue(kind, name, value)
	}
	return public.CatalogRecord{
		Kind:     public.KindCode(rec.Kind),
		ID:       rec.ID,
		Revision: rec.Revision,
		Active:   rec.Active,
		Values:   values,
		Rules:    mapCatalogRules(rec.Rules),
	}
}

func mapCatalogRules(rules []domain.CatalogRuleRecord) []public.ApplicabilityRule {
	if rules == nil {
		return nil
	}
	out := make([]public.ApplicabilityRule, len(rules))
	for i, r := range rules {
		out[i] = public.ApplicabilityRule{
			AttributeCode:        r.When.AttributeCode,
			Equals:               public.Value{Kind: public.ValueText, Text: r.When.Equals},
			Mode:                 string(r.Mode),
			IdentityParticipates: r.IdentityParticipates,
			NotApplicable:        r.NotApplicable,
			Active:               r.Active,
		}
	}
	return out
}

func (a *Adapter) mapCatalogValue(kind domain.CatalogKindCode, name string, v domain.CatalogValue) public.Value {
	fieldKind := a.fieldKind(kind, name)
	switch fieldKind {
	case domain.FieldCode:
		return public.Value{Kind: public.ValueCode, Text: v.Text}
	case domain.FieldBool:
		return public.Value{Kind: public.ValueBool, Bool: v.Bool}
	case domain.FieldInt:
		return public.Value{Kind: public.ValueInteger, Text: strconv.FormatInt(int64(v.Int), 10)}
	case domain.FieldStringList:
		return public.Value{Kind: public.ValueStringList, Strings: public.CloneStringSlice(v.List)}
	case domain.FieldRef:
		if v.Ref.Code == "" && v.Ref.Kind == "" {
			return public.Value{Kind: public.ValueReference}
		}
		return public.Value{Kind: public.ValueReference, Reference: &public.Reference{Kind: public.KindCode(v.Ref.Kind), ID: v.Ref.ID, Code: v.Ref.Code}}
	case domain.FieldEnum:
		return public.Value{Kind: public.ValueEnum, Text: v.Text}
	}
	return public.Value{Kind: public.ValueText, Text: v.Text}
}

// toDomainCatalogValues is the inverse of mapCatalogRecordSlice's per-value
// projection: it converts every public write value back to its domain
// shape, driven by the same field-kind descriptor used for reads.
func (a *Adapter) toDomainCatalogValues(kind domain.CatalogKindCode, values map[string]public.Value) map[string]domain.CatalogValue {
	out := make(map[string]domain.CatalogValue, len(values))
	for name, v := range values {
		out[name] = a.toDomainCatalogValue(kind, name, v)
	}
	return out
}

// toDomainCatalogValue is the exact inverse of mapCatalogValue.
func (a *Adapter) toDomainCatalogValue(kind domain.CatalogKindCode, name string, v public.Value) domain.CatalogValue {
	fieldKind := a.fieldKind(kind, name)
	switch fieldKind {
	case domain.FieldCode, domain.FieldText, domain.FieldEnum:
		return domain.CatalogValue{Text: v.Text}
	case domain.FieldBool:
		return domain.CatalogValue{Bool: v.Bool}
	case domain.FieldInt:
		n, _ := strconv.Atoi(v.Text)
		return domain.CatalogValue{Int: n}
	case domain.FieldStringList:
		return domain.CatalogValue{List: public.CloneStringSlice(v.Strings)}
	case domain.FieldRef:
		if v.Reference == nil {
			return domain.CatalogValue{}
		}
		return domain.CatalogValue{Ref: domain.CatalogRef{Kind: domain.CatalogKindCode(v.Reference.Kind), Code: v.Reference.Code}}
	}
	return domain.CatalogValue{Text: v.Text}
}

// toDomainCatalogRules is the exact inverse of mapCatalogRules: complete
// replacement semantics, nil versus non-nil-empty preserved.
func toDomainCatalogRules(rules []public.ApplicabilityRule) []domain.CatalogRuleRecord {
	if rules == nil {
		return nil
	}
	out := make([]domain.CatalogRuleRecord, len(rules))
	for i, r := range rules {
		out[i] = domain.CatalogRuleRecord{
			When:                 domain.AttributeCondition{AttributeCode: r.AttributeCode, Equals: r.Equals.Text},
			Mode:                 domain.AttributeMode(r.Mode),
			IdentityParticipates: r.IdentityParticipates,
			NotApplicable:        r.NotApplicable,
			Active:               r.Active,
		}
	}
	return out
}

func mapResourceQuery(criteria domain.SearchCriteria) public.ResourceQuery {
	return public.ResourceQuery{
		Scope:      publicScopeFromResourceLifecycle(criteria.LifecycleScope),
		ClassCode:  criteria.ClassCode,
		FamilyCode: criteria.FamilyCode,
		TypeCode:   criteria.TypeCode,
		Text:       criteria.Text,
		Limit:      criteria.Limit,
		Offset:     criteria.Offset,
	}
}

func publicScopeFromResourceLifecycle(s domain.ResourceLifecycleScope) public.LifecycleScope {
	if s == domain.LifecycleScopeInactive {
		return public.ScopeInactive
	}
	return public.ScopeActive
}

func mapResourceSlice(resources []domain.Resource) []public.Resource {
	out := make([]public.Resource, len(resources))
	for i, res := range resources {
		out[i] = mapResource(res)
	}
	return out
}

func mapResource(res domain.Resource) public.Resource {
	attrs := make([]public.AttributeValue, len(res.Attributes))
	for i, av := range res.Attributes {
		attrs[i] = public.AttributeValue{
			Code:     av.AttributeCode,
			UnitCode: resourceUnitCode(av),
			Value:    mapResourceAttributeValue(av),
		}
	}
	return public.Resource{
		ID:          res.ID,
		IdentityV1:  res.IdentityKey,
		Scope:       public.ResourceScope{ClassCode: res.ClassCode, FamilyCode: res.FamilyCode, TypeCode: res.TypeCode},
		NaturalUnit: res.NaturalUnit,
		Active:      res.Active,
		Revision:    res.Revision,
		Attributes:  attrs,
	}
}

func resourceUnitCode(av domain.ResourceAttributeValue) string {
	if av.Quantity != nil {
		return av.Quantity.UnitCode
	}
	return ""
}

func mapResourceAttributeValue(av domain.ResourceAttributeValue) public.Value {
	switch av.Type {
	case domain.ValueTypeControlledOption:
		return public.Value{Kind: public.ValueControlledOption, Text: av.OptionCode}
	case domain.ValueTypeInteger:
		if av.Integer == nil {
			return public.Value{Kind: public.ValueInteger}
		}
		return public.Value{Kind: public.ValueInteger, Text: strconv.FormatInt(*av.Integer, 10)}
	case domain.ValueTypeDecimal:
		if av.Decimal == nil {
			return public.Value{Kind: public.ValueDecimal}
		}
		return public.Value{Kind: public.ValueDecimal, Text: av.Decimal.String()}
	case domain.ValueTypeQuantity:
		if av.Quantity == nil {
			return public.Value{Kind: public.ValueQuantity}
		}
		return public.Value{Kind: public.ValueQuantity, Text: av.Quantity.Value.String(), UnitCode: av.Quantity.UnitCode}
	case domain.ValueTypeBoolean:
		if av.Boolean == nil {
			return public.Value{Kind: public.ValueBool}
		}
		return public.Value{Kind: public.ValueBool, Bool: *av.Boolean}
	case domain.ValueTypeControlledText:
		if av.Text == domain.NotApplicableText {
			return public.Value{Kind: public.ValueNotApplicable}
		}
		return public.Value{Kind: public.ValueText, Text: av.Text}
	}
	return public.Value{Kind: public.ValueText, Text: av.Text}
}

// toDomainResourceAttributes is the exact inverse of mapResourceSlice's
// per-attribute projection (mapResourceAttributeValue).
func toDomainResourceAttributes(attrs []public.AttributeValue) ([]domain.ResourceAttributeValue, error) {
	if attrs == nil {
		return nil, nil
	}
	out := make([]domain.ResourceAttributeValue, len(attrs))
	for i, av := range attrs {
		mapped, err := toDomainResourceAttribute(av)
		if err != nil {
			return nil, err
		}
		out[i] = mapped
	}
	return out, nil
}

// toDomainResourceAttribute is the exact inverse of mapResourceAttributeValue.
// Any Value.Kind with no resource-attribute counterpart is INVALID_ARGUMENT,
// never coerced.
func toDomainResourceAttribute(av public.AttributeValue) (domain.ResourceAttributeValue, error) {
	code := av.Code
	switch av.Value.Kind {
	case public.ValueControlledOption:
		return domain.ResourceAttributeValue{AttributeCode: code, Type: domain.ValueTypeControlledOption, OptionCode: av.Value.Text}, nil
	case public.ValueInteger:
		n, err := strconv.ParseInt(av.Value.Text, 10, 64)
		if err != nil {
			return domain.ResourceAttributeValue{}, public.NewError(public.InvalidArgument, "invalid integer attribute value")
		}
		return domain.ResourceAttributeValue{AttributeCode: code, Type: domain.ValueTypeInteger, Integer: &n}, nil
	case public.ValueDecimal:
		d, err := decimal.NewFromString(av.Value.Text)
		if err != nil {
			return domain.ResourceAttributeValue{}, public.NewError(public.InvalidArgument, "invalid decimal attribute value")
		}
		return domain.ResourceAttributeValue{AttributeCode: code, Type: domain.ValueTypeDecimal, Decimal: &d}, nil
	case public.ValueQuantity:
		d, err := decimal.NewFromString(av.Value.Text)
		if err != nil {
			return domain.ResourceAttributeValue{}, public.NewError(public.InvalidArgument, "invalid quantity attribute value")
		}
		return domain.ResourceAttributeValue{AttributeCode: code, Type: domain.ValueTypeQuantity, Quantity: &domain.Quantity{Value: d, UnitCode: av.Value.UnitCode}}, nil
	case public.ValueBool:
		b := av.Value.Bool
		return domain.ResourceAttributeValue{AttributeCode: code, Type: domain.ValueTypeBoolean, Boolean: &b}, nil
	case public.ValueNotApplicable:
		return domain.ResourceAttributeValue{AttributeCode: code, Type: domain.ValueTypeControlledText, Text: domain.NotApplicableText}, nil
	case public.ValueText:
		return domain.ResourceAttributeValue{AttributeCode: code, Type: domain.ValueTypeControlledText, Text: av.Value.Text}, nil
	}
	return domain.ResourceAttributeValue{}, public.NewError(public.InvalidArgument, "unsupported attribute value kind for a resource attribute")
}

func (a *Adapter) String() string {
	return fmt.Sprintf("resourcecore.Adapter(%p)", a)
}
