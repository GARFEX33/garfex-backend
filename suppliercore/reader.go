package suppliercore

import (
	"context"
	"strings"
)

// ReadCapabilities is the service-shaped seam a Reader delegates to.
// Implementations are expected to be authoritative for the results they
// return and to translate their own internal errors to public Error values.
type ReadCapabilities interface {
	GetSupplier(context.Context, int64) (Supplier, error)
	GetSupplierByTaxIdentifier(context.Context, string) (Supplier, error)
	SearchSuppliers(context.Context, SupplierQuery) (SupplierPage, error)
	ListBranches(context.Context, BranchQuery) (BranchPage, error)
	GetBranch(context.Context, BranchKey) (Branch, error)
	ListContacts(context.Context, ContactQuery) (ContactPage, error)
	GetContact(context.Context, ContactKey) (Contact, error)
}

// Reader is the public read-only Supplier Master contract. It validates
// request shape, delegates reads to ReadCapabilities, and defensively
// copies every returned value before exposing it.
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

// GetSupplier returns one supplier by id.
func (r *Reader) GetSupplier(ctx context.Context, id int64) (Supplier, error) {
	if id <= 0 {
		return Supplier{}, NewError(InvalidArgument, "supplier id must be positive")
	}
	s, err := r.cap.GetSupplier(ctx, id)
	if err != nil {
		return Supplier{}, err
	}
	return CloneSupplier(s), nil
}

// GetSupplierByTaxIdentifier returns the supplier owning taxID, whether it is
// active or inactive, or a NOT_FOUND error when none does. The match is exact
// and ignores case and surrounding whitespace, mirroring the uniqueness rule
// that makes a second supplier with the same tax identifier a conflict.
func (r *Reader) GetSupplierByTaxIdentifier(ctx context.Context, taxID string) (Supplier, error) {
	if strings.TrimSpace(taxID) == "" {
		return Supplier{}, NewError(InvalidArgument, "tax identifier must not be blank")
	}
	s, err := r.cap.GetSupplierByTaxIdentifier(ctx, taxID)
	if err != nil {
		return Supplier{}, err
	}
	return CloneSupplier(s), nil
}

// SearchSuppliers returns a page of suppliers matching the query.
func (r *Reader) SearchSuppliers(ctx context.Context, q SupplierQuery) (SupplierPage, error) {
	if err := validateScope(q.Scope); err != nil {
		return SupplierPage{}, err
	}
	page, err := r.cap.SearchSuppliers(ctx, q)
	if err != nil {
		return SupplierPage{}, err
	}
	page.Suppliers = cloneSupplierSlice(page.Suppliers)
	return page, nil
}

// ListBranches returns a page of branches belonging to q.SupplierID.
func (r *Reader) ListBranches(ctx context.Context, q BranchQuery) (BranchPage, error) {
	if q.SupplierID <= 0 {
		return BranchPage{}, NewError(InvalidArgument, "supplier id must be positive")
	}
	if err := validateScope(q.Scope); err != nil {
		return BranchPage{}, err
	}
	page, err := r.cap.ListBranches(ctx, q)
	if err != nil {
		return BranchPage{}, err
	}
	page.Branches = cloneBranchSlice(page.Branches)
	return page, nil
}

// GetBranch returns one branch by key.
func (r *Reader) GetBranch(ctx context.Context, key BranchKey) (Branch, error) {
	if key.SupplierID <= 0 {
		return Branch{}, NewError(InvalidArgument, "supplier id must be positive")
	}
	if key.BranchID <= 0 {
		return Branch{}, NewError(InvalidArgument, "branch id must be positive")
	}
	b, err := r.cap.GetBranch(ctx, key)
	if err != nil {
		return Branch{}, err
	}
	return CloneBranch(b), nil
}

// ListContacts returns a page of contacts belonging to q.SupplierID,
// optionally narrowed to one branch. Shape validation checks only that a
// provided BranchID is positive — whether that branch actually belongs to
// the supplier is a business rule enforced by the internal service and
// surfaces as a VALIDATION error, not a boundary rejection here.
func (r *Reader) ListContacts(ctx context.Context, q ContactQuery) (ContactPage, error) {
	if q.SupplierID <= 0 {
		return ContactPage{}, NewError(InvalidArgument, "supplier id must be positive")
	}
	if q.BranchID != nil && *q.BranchID <= 0 {
		return ContactPage{}, NewError(InvalidArgument, "branch id must be positive when provided")
	}
	if err := validateScope(q.Scope); err != nil {
		return ContactPage{}, err
	}
	page, err := r.cap.ListContacts(ctx, q)
	if err != nil {
		return ContactPage{}, err
	}
	page.Contacts = cloneContactSlice(page.Contacts)
	return page, nil
}

// GetContact returns one contact by key.
func (r *Reader) GetContact(ctx context.Context, key ContactKey) (Contact, error) {
	if key.SupplierID <= 0 {
		return Contact{}, NewError(InvalidArgument, "supplier id must be positive")
	}
	if key.ContactID <= 0 {
		return Contact{}, NewError(InvalidArgument, "contact id must be positive")
	}
	c, err := r.cap.GetContact(ctx, key)
	if err != nil {
		return Contact{}, err
	}
	return CloneContact(c), nil
}

func validateScope(s LifecycleScope) error {
	if s != "" && s != ScopeActive && s != ScopeInactive && s != ScopeAll {
		return NewError(InvalidArgument, "invalid lifecycle scope")
	}
	return nil
}
