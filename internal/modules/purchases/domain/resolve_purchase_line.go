package domain

// ResolvePurchaseLineCommand confirms the Resource mapping behind one purchase
// line. ExpectedSupplierProductID and ExpectedMappingRevision are the
// operator's authoritative line snapshot: both are nil when the line had no
// SupplierProduct association, and both are required when it did.
type ResolvePurchaseLineCommand struct {
	LineID                     int64
	ResourceID                 int64
	CommercialSupplierSKU      string
	ExpectedSupplierProductID  *int64
	ExpectedMappingRevision    *MappingRevision
	ExpectedResolutionRevision ResolutionRevision
	Actor                      string
	Reason                     string
	Decision                   MappingDecisionMetadata
}

// CommercialIdentityDisposition reports how line resolution used the
// SupplierProduct commercial identity without requiring an adapter to infer
// transactional history from the returned snapshots.
type CommercialIdentityDisposition string

const (
	CommercialIdentityCreated       CommercialIdentityDisposition = "CREATED"
	CommercialIdentityReused        CommercialIdentityDisposition = "REUSED"
	CommercialIdentityAlreadyMapped CommercialIdentityDisposition = "ALREADY_MAPPED"
)

// ResolvePurchaseLineResult returns enough authoritative state for a delivery
// adapter to answer the mutation without reconstructing Purchase Core rules.
type ResolvePurchaseLineResult struct {
	Line                          PurchaseLine
	SupplierProduct               SupplierProduct
	CommercialIdentityDisposition CommercialIdentityDisposition
}
