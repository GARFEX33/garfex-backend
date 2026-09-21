package purchasecore

import "time"

// LinkStatus is the derived or explicit resolution state of one purchase
// line. Only NONE, NO_APLICA, and CONFLICTO are persisted overrides.
type LinkStatus string

const (
	LinkStatusNone    LinkStatus = "NONE"
	LinkPending       LinkStatus = "PENDIENTE"
	LinkLinked        LinkStatus = "VINCULADO"
	LinkSuspended     LinkStatus = "SUSPENDIDO"
	LinkNotApplicable LinkStatus = "NO_APLICA"
	LinkConflict      LinkStatus = "CONFLICTO"
)

type MappingRevision uint64
type MappingState string
type MappingCause string
type MappingOrigin string
type MappingOperation string

const (
	MappingStateUnresolved       MappingState = "UNRESOLVED"
	MappingStateConfirmed        MappingState = "CONFIRMED"
	MappingStateSuspended        MappingState = "SUSPENDED"
	MappingStateIdentityConflict MappingState = "IDENTITY_CONFLICT"

	MappingCauseNone                 MappingCause = "NONE"
	MappingCauseUnresolved           MappingCause = "UNRESOLVED"
	MappingCauseResourceInactive     MappingCause = "RESOURCE_INACTIVE"
	MappingCauseIdentityConflict     MappingCause = "IDENTITY_CONFLICT"
	MappingCauseLineNotApplicable    MappingCause = "LINE_NOT_APPLICABLE"
	MappingCauseLineConflictOverride MappingCause = "LINE_CONFLICT_OVERRIDE"

	MappingOriginManual MappingOrigin = "manual"

	MappingOperationConfirm                 MappingOperation = "CONFIRM"
	MappingOperationCorrect                 MappingOperation = "CORRECT"
	MappingOperationExceptionalUnlink       MappingOperation = "EXCEPTIONAL_UNLINK"
	MappingOperationReportIdentityConflict  MappingOperation = "REPORT_IDENTITY_CONFLICT"
	MappingOperationResolveIdentityConflict MappingOperation = "RESOLVE_IDENTITY_CONFLICT"
)

type MappingDecisionMetadata struct {
	Actor  string
	Origin MappingOrigin
	Reason string
	At     time.Time
}

type SupplierProductMapping struct {
	ResourceID       *int64
	IdentityConflict bool
}

type MappingAuditEntry struct {
	SupplierProductID int64
	PreviousMapping   SupplierProductMapping
	NewMapping        SupplierProductMapping
	PreviousState     MappingState
	NewState          MappingState
	PreviousRevision  MappingRevision
	NewRevision       MappingRevision
	Operation         MappingOperation
	Decision          MappingDecisionMetadata
}

// Purchase is one fiscal purchase document (CFDI) registered in the system.
type Purchase struct {
	ID             int64
	SupplierID     int64
	BranchID       *int64
	CFDIUUID       string
	Series         string
	Folio          string
	IssuedAt       time.Time
	Currency       string
	ExchangeRate   *string
	Subtotal       string
	Discount       string
	TaxTransferred string
	TaxWithheld    string
	Total          string
	IssuerTaxID    string
	IssuerName     string
	XML            XMLDocument
	ImportedAt     time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type XMLDocument struct {
	Content  []byte
	Hash     string
	Filename string
}

type PurchaseLine struct {
	ID                 int64
	PurchaseID         int64
	LineNumber         int
	Description        string
	SupplierSKU        string
	SATProductCode     string
	Quantity           string
	UnitCode           string
	Unit               string
	UnitPrice          string
	Amount             string
	Discount           string
	TaxTransferred     string
	TaxWithheld        string
	TaxObject          string
	SupplierProductID  *int64
	ResolutionRevision ResolutionRevision
	ResolutionOverride LinkStatus
	EffectiveStatus    LinkStatus
	EffectiveCause     MappingCause
}

// PurchaseLineRow is the flattened public read model for the global
// PurchaseLine workbench. Nullable fields are represented by pointers.
type PurchaseLineRow struct {
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
	Quantity              string
	UnitCode              string
	Unit                  string
	UnitPrice             string
	Amount                string
	Currency              string
	SupplierProductID     *int64
	ResourceID            *int64
	ResourceIdentity      *string
	// ResourceDisplayName is the set-based Resource row display_name with
	// identity fallback. It is not Resource Core's enriched Describe output.
	ResourceDisplayName *string
	MappingRevision     *MappingRevision
	ResolutionRevision  ResolutionRevision
	ResolutionOverride  LinkStatus
	EffectiveStatus     LinkStatus
	EffectiveCause      MappingCause
}

type SupplierProduct struct {
	ID              int64
	SupplierID      int64
	SupplierSKU     string
	Description     string
	CurrentMapping  SupplierProductMapping
	MappingRevision MappingRevision
	// ResourceActive is a non-authoritative Resource Master read snapshot.
	ResourceActive *bool
	MappingState   MappingState
	MappingCause   MappingCause
	Notes          string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

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
