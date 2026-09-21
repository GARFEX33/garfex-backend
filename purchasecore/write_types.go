package purchasecore

// ImportRequest imports one CFDI 4.0 purchase document.
type ImportRequest struct {
	Actor    string
	XML      []byte
	BranchID int64
	Filename string
}

type ImportResult struct {
	Purchase       Purchase
	Lines          []PurchaseLine
	AlreadyExisted bool
}

type ResolutionRevision uint64

// ResolvePurchaseLineRequest confirms the mapping behind one line. When the
// observed line had no SupplierProduct, both expected mapping fields must be
// nil and CommercialSupplierSKU is required. For an existing association,
// both expected mapping fields are required and CommercialSupplierSKU is
// forbidden.
type ResolvePurchaseLineRequest struct {
	LineID                     int64
	ResourceID                 int64
	CommercialSupplierSKU      string
	ExpectedSupplierProductID  *int64
	ExpectedMappingRevision    *MappingRevision
	ExpectedResolutionRevision ResolutionRevision
	Actor                      string
	Reason                     string
}

// CommercialIdentityDisposition reports whether ResolvePurchaseLine created
// the SupplierProduct identity, reused it while changing its mapping, or found
// it already mapped to the requested active Resource.
type CommercialIdentityDisposition string

const (
	CommercialIdentityCreated       CommercialIdentityDisposition = "CREATED"
	CommercialIdentityReused        CommercialIdentityDisposition = "REUSED"
	CommercialIdentityAlreadyMapped CommercialIdentityDisposition = "ALREADY_MAPPED"
)

type ResolvePurchaseLineResult struct {
	Line                          PurchaseLine
	SupplierProduct               SupplierProduct
	CommercialIdentityDisposition CommercialIdentityDisposition
}

// SetResolutionOverrideRequest stores only NONE, NO_APLICA, or CONFLICTO.
// Actor and Reason are required for every manual decision, including NONE.
type SetResolutionOverrideRequest struct {
	LineID           int64
	Override         LinkStatus
	ExpectedRevision ResolutionRevision
	Actor            string
	Reason           string
}

type ConfirmMappingRequest struct {
	SupplierProductID int64
	ResourceID        int64
	ExpectedRevision  MappingRevision
	Decision          MappingDecisionMetadata
}

type CorrectMappingRequest struct {
	SupplierProductID         int64
	ExpectedCurrentResourceID int64
	ResourceID                int64
	ExpectedRevision          MappingRevision
	Decision                  MappingDecisionMetadata
}

type ExceptionalUnlinkRequest struct {
	SupplierProductID         int64
	ExpectedCurrentResourceID int64
	ExpectedRevision          MappingRevision
	Decision                  MappingDecisionMetadata
}

type ReportIdentityConflictRequest struct {
	SupplierProductID         int64
	ExpectedCurrentResourceID int64
	ExpectedRevision          MappingRevision
	Decision                  MappingDecisionMetadata
}

type ResolveIdentityConflictRequest struct {
	SupplierProductID         int64
	ExpectedCurrentResourceID int64
	ResourceID                int64
	ExpectedRevision          MappingRevision
	Decision                  MappingDecisionMetadata
}
