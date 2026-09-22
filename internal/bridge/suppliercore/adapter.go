// Package suppliercore is the internal bridge between the public
// suppliercore contract (read, plus the graduated Supplier Create and
// Update writes) and the authoritative Supplier Master application service. It translates
// public requests to domain calls, maps internal errors to public errors,
// and defensively copies data crossing the boundary. It never re-implements
// a business rule already owned by the internal service — most notably
// branch ownership, which this bridge only observes the result of.
package suppliercore

import (
	"context"
	"errors"

	"github.com/GARFEX33/garfex-backend/internal/core"
	"github.com/GARFEX33/garfex-backend/internal/modules/suppliers/domain"
	public "github.com/GARFEX33/garfex-backend/suppliercore"
)

// defaultLimit matches the internal postgres repository's own private
// default (internal/modules/suppliers/postgres/repository.go). Declared
// again here because the bridge must resolve a query's effective limit
// itself, before deciding how to over-fetch for pagination — see
// effectiveLimit.
const defaultLimit = 100

// serviceReader is the narrow seam this bridge depends on: exactly the 6
// read methods public.ReadCapabilities needs, out of
// suppliers/app.Service's full surface. A single seam (not one per
// aggregate, unlike resourcecore's catalogReader/resourceReader split) is
// correct here because one internal Service struct — not several distinct
// services — already backs all three aggregates.
type serviceReader interface {
	GetSupplier(ctx context.Context, id int64) (domain.Supplier, error)
	GetSupplierByTaxIdentifier(ctx context.Context, taxID string) (domain.Supplier, error)
	SearchSuppliers(ctx context.Context, criteria domain.SupplierSearch) ([]domain.Supplier, error)
	GetBranch(ctx context.Context, supplierID, branchID int64) (domain.Branch, error)
	ListBranches(ctx context.Context, supplierID int64, criteria domain.ListCriteria) ([]domain.Branch, error)
	GetContact(ctx context.Context, supplierID, contactID int64) (domain.Contact, error)
	ListContacts(ctx context.Context, supplierID int64, criteria domain.ContactListCriteria) ([]domain.Contact, error)
}

// serviceWriter is the narrow seam this bridge depends on for graduated
// write operations. Only CreateSupplier and UpdateSupplier are graduated so
// far.
type serviceWriter interface {
	CreateSupplier(ctx context.Context, details domain.SupplierDetails) (domain.Supplier, error)
	UpdateSupplier(ctx context.Context, id int64, details domain.SupplierDetails) (domain.Supplier, error)
}

// supplierService is the combined seam: one internal Service struct backs
// all three aggregates and both capability directions, so a single
// combined interface is correct here — unlike resourcecore's separate
// catalogReader/catalogWriter split, which exists because Catalog and
// Resource are backed by two distinct internal services.
type supplierService interface {
	serviceReader
	serviceWriter
}

// Adapter implements public.ReadCapabilities and public.WriteCapabilities
// over an internal supplierService.
type Adapter struct {
	service supplierService
}

var _ public.ReadCapabilities = (*Adapter)(nil)
var _ public.WriteCapabilities = (*Adapter)(nil)

// NewAdapter returns a ReadCapabilities and WriteCapabilities
// implementation backed by service. service may not be nil.
func NewAdapter(service supplierService) *Adapter {
	return &Adapter{service: service}
}

// CreateSupplier creates one supplier. req.Actor is diagnostic-only
// attribution, carried via core.WithActor; it is never persisted and never
// a business parameter.
func (a *Adapter) CreateSupplier(ctx context.Context, req public.SupplierWriteRequest) (public.Supplier, error) {
	ctx = core.WithActor(ctx, req.Actor)
	created, err := a.service.CreateSupplier(ctx, domain.SupplierDetails{
		TradeName:     req.TradeName,
		LegalName:     req.LegalName,
		TaxIdentifier: req.TaxIdentifier,
		Website:       req.Website,
		Notes:         req.Notes,
	})
	if err != nil {
		return public.Supplier{}, mapError(err)
	}
	return mapSupplier(created), nil
}

// UpdateSupplier replaces one existing supplier's content. req.Actor is
// diagnostic-only attribution, carried via core.WithActor; req.ID and the
// mapped domain.SupplierDetails are the only business inputs.
func (a *Adapter) UpdateSupplier(ctx context.Context, req public.SupplierUpdateRequest) (public.Supplier, error) {
	ctx = core.WithActor(ctx, req.Actor)
	updated, err := a.service.UpdateSupplier(ctx, req.ID, domain.SupplierDetails{
		TradeName:     req.TradeName,
		LegalName:     req.LegalName,
		TaxIdentifier: req.TaxIdentifier,
		Website:       req.Website,
		Notes:         req.Notes,
	})
	if err != nil {
		return public.Supplier{}, mapError(err)
	}
	return mapSupplier(updated), nil
}

// GetSupplier returns one supplier by id.
func (a *Adapter) GetSupplier(ctx context.Context, id int64) (public.Supplier, error) {
	s, err := a.service.GetSupplier(ctx, id)
	if err != nil {
		return public.Supplier{}, mapError(err)
	}
	return mapSupplier(s), nil
}

// GetSupplierByTaxIdentifier returns the one supplier owning taxID, active or not.
func (a *Adapter) GetSupplierByTaxIdentifier(ctx context.Context, taxID string) (public.Supplier, error) {
	s, err := a.service.GetSupplierByTaxIdentifier(ctx, taxID)
	if err != nil {
		return public.Supplier{}, mapError(err)
	}
	return mapSupplier(s), nil
}

// SearchSuppliers returns a page of suppliers matching q.
//
// Unlike Branch/Contact, the internal supplier repository does not
// over-fetch internally (it uses a plain LIMIT, not LIMIT+1), so this
// bridge must explicitly request one extra row itself to derive HasNext —
// exactly the technique resourcecore's ListCatalog uses. That request must
// use the already-resolved effectiveLimit, not the raw q.Limit: adding 1 to
// a caller's q.Limit == 0 (meaning "use the default page size") would ask
// for a 1-row page instead of the intended default-sized one.
func (a *Adapter) SearchSuppliers(ctx context.Context, q public.SupplierQuery) (public.SupplierPage, error) {
	limit := effectiveLimit(q.Limit)
	suppliers, err := a.service.SearchSuppliers(ctx, domain.SupplierSearch{
		Text:   q.Text,
		Active: activeFromScope(q.Scope),
		Limit:  limit + 1,
		Offset: q.Offset,
	})
	if err != nil {
		return public.SupplierPage{}, mapError(err)
	}
	end, hasNext := trimToPage(len(suppliers), limit)
	return public.SupplierPage{
		Query:       public.SupplierQuery{Text: q.Text, Scope: q.Scope, Limit: limit, Offset: q.Offset},
		Suppliers:   mapSupplierSlice(suppliers[:end]),
		HasPrevious: q.Offset > 0,
		HasNext:     hasNext,
	}, nil
}

// effectiveLimit resolves the page size a query actually wants: a
// non-positive requested limit means "use the default", mirroring the
// internal postgres repository's own limit() helper.
func effectiveLimit(requested int) int {
	if requested <= 0 {
		return defaultLimit
	}
	return requested
}

// trimToPage caps results at limit and reports whether more exist beyond
// it.
func trimToPage(results int, limit int) (end int, hasNext bool) {
	if results > limit {
		return limit, true
	}
	return results, false
}

func activeFromScope(scope public.LifecycleScope) *bool {
	switch scope {
	case public.ScopeInactive:
		inactive := false
		return &inactive
	case public.ScopeAll:
		return nil
	default:
		active := true
		return &active
	}
}

func mapSupplier(s domain.Supplier) public.Supplier {
	return public.Supplier{
		ID:            s.ID,
		TradeName:     s.TradeName,
		LegalName:     s.LegalName,
		TaxIdentifier: s.TaxIdentifier,
		Website:       s.Website,
		Notes:         s.Notes,
		Active:        s.Active,
		CreatedAt:     s.CreatedAt,
		UpdatedAt:     s.UpdatedAt,
	}
}

func mapSupplierSlice(suppliers []domain.Supplier) []public.Supplier {
	out := make([]public.Supplier, len(suppliers))
	for i := range suppliers {
		out[i] = mapSupplier(suppliers[i])
	}
	return out
}

// ListBranches returns a page of branches belonging to q.SupplierID. Unlike
// SearchSuppliers, the internal branch repository already over-fetches by
// one row internally (postgres uses LIMIT+1), so the bridge requests
// effectiveLimit unmodified rather than effectiveLimit+1.
func (a *Adapter) ListBranches(ctx context.Context, q public.BranchQuery) (public.BranchPage, error) {
	limit := effectiveLimit(q.Limit)
	branches, err := a.service.ListBranches(ctx, q.SupplierID, domain.ListCriteria{
		Text:   q.Text,
		Active: activeFromScope(q.Scope),
		Limit:  limit,
		Offset: q.Offset,
	})
	if err != nil {
		return public.BranchPage{}, mapError(err)
	}
	end, hasNext := trimToPage(len(branches), limit)
	return public.BranchPage{
		Query:       public.BranchQuery{SupplierID: q.SupplierID, Text: q.Text, Scope: q.Scope, Limit: limit, Offset: q.Offset},
		Branches:    mapBranchSlice(branches[:end]),
		HasPrevious: q.Offset > 0,
		HasNext:     hasNext,
	}, nil
}

// GetBranch returns one branch by key.
func (a *Adapter) GetBranch(ctx context.Context, key public.BranchKey) (public.Branch, error) {
	b, err := a.service.GetBranch(ctx, key.SupplierID, key.BranchID)
	if err != nil {
		return public.Branch{}, mapError(err)
	}
	return mapBranch(b), nil
}

// ListContacts returns a page of contacts belonging to q.SupplierID,
// optionally narrowed to one branch. Same over-fetch strategy as
// ListBranches. A foreign-branch filter surfaces as VALIDATION through the
// internal service's own ownership check (app/contact.go's
// ensureBranchOwnership) — this bridge does not re-implement that rule.
func (a *Adapter) ListContacts(ctx context.Context, q public.ContactQuery) (public.ContactPage, error) {
	limit := effectiveLimit(q.Limit)
	contacts, err := a.service.ListContacts(ctx, q.SupplierID, domain.ContactListCriteria{
		Text:     q.Text,
		Active:   activeFromScope(q.Scope),
		BranchID: q.BranchID,
		Limit:    limit,
		Offset:   q.Offset,
	})
	if err != nil {
		return public.ContactPage{}, mapError(err)
	}
	end, hasNext := trimToPage(len(contacts), limit)
	return public.ContactPage{
		Query:       public.ContactQuery{SupplierID: q.SupplierID, BranchID: q.BranchID, Text: q.Text, Scope: q.Scope, Limit: limit, Offset: q.Offset},
		Contacts:    mapContactSlice(contacts[:end]),
		HasPrevious: q.Offset > 0,
		HasNext:     hasNext,
	}, nil
}

// GetContact returns one contact by key.
func (a *Adapter) GetContact(ctx context.Context, key public.ContactKey) (public.Contact, error) {
	c, err := a.service.GetContact(ctx, key.SupplierID, key.ContactID)
	if err != nil {
		return public.Contact{}, mapError(err)
	}
	return mapContact(c), nil
}

func mapBranch(b domain.Branch) public.Branch {
	return public.Branch{
		ID:           b.ID,
		SupplierID:   b.SupplierID,
		Name:         b.Name,
		Reference:    b.Reference,
		City:         b.City,
		State:        b.State,
		Country:      b.Country,
		Address:      b.Address,
		GeneralPhone: b.GeneralPhone,
		GeneralEmail: b.GeneralEmail,
		Notes:        b.Notes,
		Active:       b.Active,
		CreatedAt:    b.CreatedAt,
		UpdatedAt:    b.UpdatedAt,
	}
}

func mapBranchSlice(branches []domain.Branch) []public.Branch {
	out := make([]public.Branch, len(branches))
	for i := range branches {
		out[i] = mapBranch(branches[i])
	}
	return out
}

func mapContact(c domain.Contact) public.Contact {
	return public.Contact{
		ID:         c.ID,
		SupplierID: c.SupplierID,
		BranchID:   copyBranchID(c.BranchID),
		Name:       c.Name,
		Role:       c.Role,
		Phone:      c.Phone,
		Mobile:     c.Mobile,
		Email:      c.Email,
		Notes:      c.Notes,
		Active:     c.Active,
		CreatedAt:  c.CreatedAt,
		UpdatedAt:  c.UpdatedAt,
	}
}

func copyBranchID(id *int64) *int64 {
	if id == nil {
		return nil
	}
	v := *id
	return &v
}

func mapContactSlice(contacts []domain.Contact) []public.Contact {
	out := make([]public.Contact, len(contacts))
	for i := range contacts {
		out[i] = mapContact(contacts[i])
	}
	return out
}

// mapError classifies an internal Supplier Master error into a public
// Error, for both the read and write paths — a single mapper, not a
// sibling per direction, since the internal error taxonomy (ErrNotFound,
// ErrBranchOwnership, ErrValidation, ErrConflict) is the same on both
// sides. The default branch always carries a fixed, generic message —
// never the original error's text — so a raw, unsanitized internal error
// can never leak PostgreSQL detail through this bridge.
func mapError(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return public.NewError(public.NotFound, "supplier master record not found")
	case errors.Is(err, domain.ErrBranchOwnership):
		return public.NewError(public.Validation, "branch does not belong to supplier")
	case errors.Is(err, domain.ErrValidation):
		return public.NewError(public.Validation, "supplier master validation failed")
	case errors.Is(err, domain.ErrConflict):
		return public.NewError(public.Conflict, "supplier master conflict")
	default:
		return public.NewError(public.Internal, "supplier master operation failed")
	}
}
