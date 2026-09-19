package resourcecore

// CatalogWriteRequest creates one record of any registered catalog kind.
type CatalogWriteRequest struct {
	Actor  string
	Kind   KindCode
	Active bool
	Values map[string]Value
	Rules  []ApplicabilityRule // APLICABILIDAD aggregate; nil and empty differ
}

// ResourceWriteRequest creates one resource.
type ResourceWriteRequest struct {
	Actor       string
	Scope       ResourceScope
	NaturalUnit string
	Attributes  []AttributeValue
}

// CatalogUpdateRequest replaces one existing catalog record of any
// registered kind under optimistic concurrency.
type CatalogUpdateRequest struct {
	Actor            string
	Kind             KindCode
	ID               int64
	ExpectedRevision uint64
	Active           bool
	Values           map[string]Value
	Rules            []ApplicabilityRule // APLICABILIDAD aggregate; nil and empty differ
}

// ResourceUpdateRequest replaces one existing resource under optimistic
// concurrency. ID is a stable BIGSERIAL primary key, obtainable today only
// as a byproduct of a prior GetResource/SearchResources call.
type ResourceUpdateRequest struct {
	Actor            string
	ID               int64
	ExpectedRevision uint64
	Scope            ResourceScope
	NaturalUnit      string
	Attributes       []AttributeValue
}

// CatalogLifecycleRequest deactivates, reactivates, or hard-deletes one
// existing catalog record of any registered kind under optimistic
// concurrency. No content fields — none of these operations touch
// Values/Rules.
type CatalogLifecycleRequest struct {
	Actor            string
	Kind             KindCode
	ID               int64
	ExpectedRevision uint64
}

// ResourceLifecycleRequest deactivates or reactivates one existing resource
// under optimistic concurrency. No content fields.
type ResourceLifecycleRequest struct {
	Actor            string
	ID               int64
	ExpectedRevision uint64
}

// AttributeOrderWriteRequest replaces one existing resource type's effective
// attribute presentation order under optimistic concurrency.
// ExpectedOrderRevision must equal the OrderRevision most recently observed
// from Reader.AttributeOrderFor (or a prior UpdateAttributeOrder result) for
// the exact same scope; OrderedAttributes must be an exact permutation of
// that same snapshot's OrderedAttributes. Actor is audit metadata, not
// authentication.
type AttributeOrderWriteRequest struct {
	Actor                 string
	Scope                 ResourceScope
	ExpectedOrderRevision string
	OrderedAttributes     []AttributeOrderKey
}
