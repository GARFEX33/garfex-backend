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
