package purchasecore

// ImportRequest imports one CFDI 4.0 purchase document from its raw XML
// bytes. Actor is diagnostic-only attribution; it is never a business
// parameter. BranchID is optional: zero means the document carries no
// applicable branch. Filename is informational traceability only.
type ImportRequest struct {
	Actor    string
	XML      []byte
	BranchID int64
	Filename string
}

// ImportResult is the outcome of importing one purchase document.
type ImportResult struct {
	Purchase Purchase
	Lines    []PurchaseLine
	// AlreadyExisted is true when the document's fiscal UUID was already
	// registered with matching content; no additional write happened.
	AlreadyExisted bool
}

// LinkSupplierProductRequest relates one SupplierProduct to one Resource
// Master entry.
type LinkSupplierProductRequest struct {
	Actor             string
	SupplierProductID int64
	ResourceID        int64
}

// UnlinkSupplierProductRequest removes a SupplierProduct's Resource Master
// relation.
type UnlinkSupplierProductRequest struct {
	Actor             string
	SupplierProductID int64
}

// SetPurchaseLineLinkStatusRequest manually overrides one line's linking
// state.
type SetPurchaseLineLinkStatusRequest struct {
	Actor          string
	PurchaseLineID int64
	Status         LinkStatus
}
