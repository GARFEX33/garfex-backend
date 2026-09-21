package domain

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

// ImportResult is the outcome of importing one purchase document.
type ImportResult struct {
	Purchase       Purchase
	Lines          []PurchaseLine
	AlreadyExisted bool
}

// ListCriteria paginates and optionally windows a history query.
type ListCriteria struct {
	Limit  int
	Offset int
}

// PurchaseLineWorkbenchCriteria filters the global PurchaseLine workbench.
// SupplierID is optional; status and date windows are applied to the derived
// read projection, never to persisted presentation state.
type PurchaseLineWorkbenchCriteria struct {
	Limit           int
	Offset          int
	SupplierID      *int64
	EffectiveStatus LinkStatus
	DateFrom        *time.Time
	DateTo          *time.Time
	InvoiceText     string
	SupplierSKU     string
	Description     string
}

// PurchaseLineWorkbenchRow is the flattened read projection used by the
// global PurchaseLine workbench. Resource and commercial supplier fields are
// nullable because a line may not have a current SupplierProduct mapping.
type PurchaseLineWorkbenchRow struct {
	LineID                int64
	LineNumber            int
	PurchaseID            int64
	IssuedAt              time.Time
	Series                string
	Folio                 string
	CFDIUUID              string
	SupplierID            int64
	SupplierDisplayName   string
	Description           string
	SupplierSKU           string
	CommercialSupplierSKU *string
	SATProductCode        string
	Quantity              decimal.Decimal
	UnitCode              string
	Unit                  string
	UnitPrice             decimal.Decimal
	Amount                decimal.Decimal
	Currency              string
	SupplierProductID     *int64
	ResourceID            *int64
	ResourceIdentity      *string
	// ResourceDisplayName is a set-based display_name projection with the
	// Resource identity as fallback, not the enriched catalog Describe value.
	ResourceDisplayName *string
	MappingRevision     *MappingRevision
	ResolutionRevision  ResolutionRevision
	ResolutionOverride  LinkStatus
	EffectiveStatus     LinkStatus
	EffectiveCause      MappingCause
}

// Repository is the persistence port for the Purchase and Price History
// core.
type Repository interface {
	MappingRepository

	// Import atomically persists draft: the purchase, all of its lines, and
	// the supplier-product relations they resolve to, or persists nothing.
	//
	// If a purchase with draft.CFDIUUID is already registered, no write
	// happens: the existing purchase and its lines are returned with
	// AlreadyExisted true when their relevant content matches draft, or
	// ErrPurchaseConflict is returned when it does not. Concurrent imports
	// of the same document are safe: the database's uniqueness guarantee on
	// the fiscal UUID is the final arbiter, not an in-process check.
	Import(context.Context, PurchaseDraft) (ImportResult, error)

	GetPurchase(context.Context, int64) (Purchase, error)
	GetPurchaseByUUID(context.Context, string) (Purchase, error)
	ListPurchaseLines(context.Context, int64) ([]PurchaseLine, error)
	ListPurchaseLinesWorkbench(context.Context, PurchaseLineWorkbenchCriteria) ([]PurchaseLineWorkbenchRow, error)
	// ListPurchasesBySupplier supports the "what has been bought from this
	// supplier" history view, most recent first.
	ListPurchasesBySupplier(context.Context, int64, ListCriteria) ([]Purchase, error)

	GetSupplierProduct(context.Context, int64) (SupplierProduct, error)
	FindSupplierProduct(ctx context.Context, supplierID int64, sku string) (SupplierProduct, error)
	ListSupplierProducts(ctx context.Context, supplierID int64, criteria ListCriteria) ([]SupplierProduct, error)

	// SetResolutionOverride is the sole manual command for storing NONE,
	// NO_APLICA, or CONFLICTO. Derived effective states are never accepted.
	SetResolutionOverride(context.Context, SetResolutionOverrideCommand) (PurchaseLine, error)

	// ResolvePurchaseLine atomically validates the line snapshot, creates or
	// reuses its SupplierProduct identity when needed, associates the line, and
	// confirms the SupplierProduct mapping through the aggregate transition.
	ResolvePurchaseLine(context.Context, ResolvePurchaseLineCommand) (ResolvePurchaseLineResult, error)

	// ListPurchaseLinesByResource supports the "who has sold this resource,
	// at what price, when" history view, most recent purchase first. It
	// joins through each line's SupplierProduct relation, so it reflects the
	// current linking state rather than a value frozen at import time.
	ListPurchaseLinesByResource(ctx context.Context, resourceID int64, criteria ListCriteria) ([]PurchaseLineHistory, error)
}

// PurchaseLineHistory is one historical purchase fact joined with enough
// purchase-level context to build a price-history or comparison view,
// without exposing the full Purchase or SupplierProduct aggregates.
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
