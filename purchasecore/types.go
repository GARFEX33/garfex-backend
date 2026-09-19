package purchasecore

import "time"

// LinkStatus is the functional linking state of one PurchaseLine toward a
// SupplierProduct and, through it, a Resource Master entry.
type LinkStatus string

const (
	LinkPending       LinkStatus = "PENDIENTE"
	LinkLinked        LinkStatus = "VINCULADO"
	LinkNotApplicable LinkStatus = "NO_APLICA"
	LinkConflict      LinkStatus = "CONFLICTO"
)

// Purchase is one fiscal purchase document (CFDI) registered in the
// system. Every field mirrors data taken from the original XML; none of it
// is recomputed or normalized here.
type Purchase struct {
	ID         int64
	SupplierID int64
	// BranchID is nil when the document carries no applicable branch.
	BranchID *int64

	CFDIUUID string
	Series   string
	Folio    string
	// IssuedAt is the CFDI's own issuance date (Comprobante Fecha), the
	// date every price-history and evolution query anchors on. It is never
	// the import timestamp.
	IssuedAt       time.Time
	Currency       string
	ExchangeRate   *string
	Subtotal       string
	Discount       string
	TaxTransferred string
	TaxWithheld    string
	Total          string

	IssuerTaxID string
	IssuerName  string

	XML XMLDocument

	ImportedAt time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// XMLDocument preserves the original purchase document integrally.
type XMLDocument struct {
	Content []byte
	// Hash is the lower-case hex SHA-256 of Content.
	Hash string
	// Filename is informational traceability only; it is never part of any
	// document's identity.
	Filename string
}

// PurchaseLine is one concept of a Purchase, preserved as a historical
// fact. Its original fields never change after import; only
// SupplierProductID and LinkStatus change, and only as an explicit
// relation correction.
type PurchaseLine struct {
	ID             int64
	PurchaseID     int64
	LineNumber     int
	Description    string
	SupplierSKU    string
	SATProductCode string
	Quantity       string
	UnitCode       string
	Unit           string
	UnitPrice      string
	Amount         string
	Discount       string
	TaxTransferred string
	TaxWithheld    string
	TaxObject      string

	SupplierProductID *int64
	LinkStatus        LinkStatus
}

// SupplierProduct is the reusable commercial identity of one article sold
// by one supplier: Supplier + supplier SKU. Its optional ResourceID links
// it to a Resource Master entry.
type SupplierProduct struct {
	ID          int64
	SupplierID  int64
	SupplierSKU string
	// Description is the last-seen description for this SKU. It is
	// informational only, never authoritative.
	Description string
	ResourceID  *int64
	Notes       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// PurchaseLineHistory is one historical purchase fact joined with enough
// purchase-level context to build a price-history or comparison view. It
// reflects the current linking state, not one frozen at import time.
type PurchaseLineHistory struct {
	Line            PurchaseLine
	SupplierProduct SupplierProduct
	PurchaseID      int64
	PurchaseUUID    string
	SupplierID      int64
	BranchID        *int64
	IssuedAt        time.Time
	Currency        string
}
