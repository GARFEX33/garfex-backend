package suppliercore

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/GARFEX33/garfex-backend/internal/core"
	"github.com/GARFEX33/garfex-backend/internal/modules/suppliers/domain"
	public "github.com/GARFEX33/garfex-backend/suppliercore"
)

type stubService struct {
	getSupplier     func(ctx context.Context, id int64) (domain.Supplier, error)
	getByTaxID      func(ctx context.Context, taxID string) (domain.Supplier, error)
	searchSuppliers func(ctx context.Context, criteria domain.SupplierSearch) ([]domain.Supplier, error)
	getBranch       func(ctx context.Context, supplierID, branchID int64) (domain.Branch, error)
	listBranches    func(ctx context.Context, supplierID int64, criteria domain.ListCriteria) ([]domain.Branch, error)
	getContact      func(ctx context.Context, supplierID, contactID int64) (domain.Contact, error)
	listContacts    func(ctx context.Context, supplierID int64, criteria domain.ContactListCriteria) ([]domain.Contact, error)
	createSupplier  func(ctx context.Context, details domain.SupplierDetails) (domain.Supplier, error)
	updateSupplier  func(ctx context.Context, id int64, details domain.SupplierDetails) (domain.Supplier, error)
}

func (s *stubService) GetSupplier(ctx context.Context, id int64) (domain.Supplier, error) {
	return s.getSupplier(ctx, id)
}
func (s *stubService) GetSupplierByTaxIdentifier(ctx context.Context, taxID string) (domain.Supplier, error) {
	return s.getByTaxID(ctx, taxID)
}
func (s *stubService) SearchSuppliers(ctx context.Context, criteria domain.SupplierSearch) ([]domain.Supplier, error) {
	return s.searchSuppliers(ctx, criteria)
}
func (s *stubService) GetBranch(ctx context.Context, supplierID, branchID int64) (domain.Branch, error) {
	return s.getBranch(ctx, supplierID, branchID)
}
func (s *stubService) ListBranches(ctx context.Context, supplierID int64, criteria domain.ListCriteria) ([]domain.Branch, error) {
	return s.listBranches(ctx, supplierID, criteria)
}
func (s *stubService) GetContact(ctx context.Context, supplierID, contactID int64) (domain.Contact, error) {
	return s.getContact(ctx, supplierID, contactID)
}
func (s *stubService) ListContacts(ctx context.Context, supplierID int64, criteria domain.ContactListCriteria) ([]domain.Contact, error) {
	return s.listContacts(ctx, supplierID, criteria)
}
func (s *stubService) CreateSupplier(ctx context.Context, details domain.SupplierDetails) (domain.Supplier, error) {
	return s.createSupplier(ctx, details)
}
func (s *stubService) UpdateSupplier(ctx context.Context, id int64, details domain.SupplierDetails) (domain.Supplier, error) {
	return s.updateSupplier(ctx, id, details)
}

func TestAdapter_GetSupplier_MapsAllFields(t *testing.T) {
	want := domain.Supplier{ID: 1, TradeName: "ACME", LegalName: "ACME SA", TaxIdentifier: "TAX1", Website: "acme.test", Notes: "n", Active: true}
	stub := &stubService{getSupplier: func(ctx context.Context, id int64) (domain.Supplier, error) { return want, nil }}
	adapter := NewAdapter(stub)

	got, err := adapter.GetSupplier(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetSupplier error = %v", err)
	}

	// Field-completeness: every domain.Supplier field must be reflected in
	// the public mapping (fails loudly if a future domain field is added
	// and forgotten here).
	domainFields := reflect.VisibleFields(reflect.TypeOf(domain.Supplier{}))
	publicVal := reflect.ValueOf(got)
	for _, f := range domainFields {
		if !publicVal.FieldByName(f.Name).IsValid() {
			t.Errorf("domain.Supplier field %s has no corresponding public.Supplier field", f.Name)
		}
	}
	if got.TradeName != want.TradeName || got.TaxIdentifier != want.TaxIdentifier {
		t.Fatalf("mapSupplier mismatch: got %+v, want fields from %+v", got, want)
	}
}

func TestAdapter_GetSupplier_NotFound(t *testing.T) {
	stub := &stubService{getSupplier: func(ctx context.Context, id int64) (domain.Supplier, error) {
		return domain.Supplier{}, domain.ErrSupplierNotFound
	}}
	adapter := NewAdapter(stub)

	_, err := adapter.GetSupplier(context.Background(), 999)
	if !public.IsCode(err, public.NotFound) {
		t.Fatalf("GetSupplier error code = %v, want %v", public.Code(err), public.NotFound)
	}
}

func TestAdapter_GetSupplier_ConflictClassifies(t *testing.T) {
	stub := &stubService{getSupplier: func(ctx context.Context, id int64) (domain.Supplier, error) {
		return domain.Supplier{}, domain.ErrTaxIdentifierConflict
	}}
	adapter := NewAdapter(stub)

	_, err := adapter.GetSupplier(context.Background(), 1)
	if !public.IsCode(err, public.Conflict) {
		t.Fatalf("GetSupplier error code = %v, want %v", public.Code(err), public.Conflict)
	}
}

func TestAdapter_GetSupplier_RawInternalErrorNeverLeaks(t *testing.T) {
	rawErr := errors.New(`ERROR: duplicate key value violates unique constraint "suppliers_tax_identifier_key" (SQLSTATE 23505)`)
	stub := &stubService{getSupplier: func(ctx context.Context, id int64) (domain.Supplier, error) { return domain.Supplier{}, rawErr }}
	adapter := NewAdapter(stub)

	_, err := adapter.GetSupplier(context.Background(), 1)
	if !public.IsCode(err, public.Internal) {
		t.Fatalf("GetSupplier error code = %v, want %v", public.Code(err), public.Internal)
	}
	if err.Error() == rawErr.Error() {
		t.Fatal("public error message leaked the raw internal error text")
	}
	for _, forbidden := range []string{"SQLSTATE", "23505", "suppliers_tax_identifier_key", "duplicate key"} {
		if strings.Contains(err.Error(), forbidden) {
			t.Errorf("public error message %q leaks driver detail %q", err.Error(), forbidden)
		}
	}
}

func TestAdapter_SearchSuppliers_DefaultLimitDoesNotUnderFetch(t *testing.T) {
	// Regression test for the caught pagination bug: a caller passing
	// Limit: 0 (meaning "use the default") must not receive a 1-row page
	// because the bridge naively added 1 to a zero limit.
	full := make([]domain.Supplier, defaultLimit)
	for i := range full {
		full[i] = domain.Supplier{ID: int64(i + 1)}
	}
	stub := &stubService{searchSuppliers: func(ctx context.Context, criteria domain.SupplierSearch) ([]domain.Supplier, error) {
		if criteria.Limit != defaultLimit+1 {
			t.Fatalf("internal search called with Limit = %d, want %d", criteria.Limit, defaultLimit+1)
		}
		return full, nil
	}}
	adapter := NewAdapter(stub)

	page, err := adapter.SearchSuppliers(context.Background(), public.SupplierQuery{Limit: 0})
	if err != nil {
		t.Fatalf("SearchSuppliers error = %v", err)
	}
	if len(page.Suppliers) != defaultLimit {
		t.Fatalf("SearchSuppliers(Limit: 0) returned %d suppliers, want %d", len(page.Suppliers), defaultLimit)
	}
}

func TestAdapter_SearchSuppliers_CapsAtLimitAndReportsHasNext(t *testing.T) {
	const limit = 5
	overFetched := make([]domain.Supplier, limit+1)
	stub := &stubService{searchSuppliers: func(ctx context.Context, criteria domain.SupplierSearch) ([]domain.Supplier, error) {
		return overFetched, nil
	}}
	adapter := NewAdapter(stub)

	page, err := adapter.SearchSuppliers(context.Background(), public.SupplierQuery{Limit: limit})
	if err != nil {
		t.Fatalf("SearchSuppliers error = %v", err)
	}
	if len(page.Suppliers) != limit {
		t.Fatalf("SearchSuppliers page length = %d, want %d", len(page.Suppliers), limit)
	}
	if !page.HasNext {
		t.Fatal("SearchSuppliers HasNext = false, want true")
	}
}

func TestAdapter_SearchSuppliers_HasPreviousReflectsOffset(t *testing.T) {
	stub := &stubService{searchSuppliers: func(ctx context.Context, criteria domain.SupplierSearch) ([]domain.Supplier, error) {
		return nil, nil
	}}
	adapter := NewAdapter(stub)

	page, err := adapter.SearchSuppliers(context.Background(), public.SupplierQuery{Offset: 10})
	if err != nil {
		t.Fatalf("SearchSuppliers error = %v", err)
	}
	if !page.HasPrevious {
		t.Fatal("SearchSuppliers HasPrevious = false, want true for Offset > 0")
	}
}

func TestAdapter_ImplementsReadCapabilities(t *testing.T) {
	var _ public.ReadCapabilities = NewAdapter(&stubService{})
}

func TestAdapter_ListBranches_RequestsEffectiveLimitUnmodified(t *testing.T) {
	// Unlike SearchSuppliers, ListBranches must NOT add 1 itself: the
	// internal postgres branch repository already over-fetches by one row
	// (LIMIT+1) regardless of what limit it's asked for.
	const limit = 5
	overFetched := make([]domain.Branch, limit+1)
	stub := &stubService{listBranches: func(ctx context.Context, supplierID int64, criteria domain.ListCriteria) ([]domain.Branch, error) {
		if criteria.Limit != limit {
			t.Fatalf("internal ListBranches called with Limit = %d, want %d", criteria.Limit, limit)
		}
		return overFetched, nil
	}}
	adapter := NewAdapter(stub)

	page, err := adapter.ListBranches(context.Background(), public.BranchQuery{SupplierID: 1, Limit: limit})
	if err != nil {
		t.Fatalf("ListBranches error = %v", err)
	}
	if len(page.Branches) != limit || !page.HasNext {
		t.Fatalf("ListBranches page = %d entries, HasNext = %v; want %d entries, HasNext = true", len(page.Branches), page.HasNext, limit)
	}
}

func TestAdapter_ListContacts_RequestsEffectiveLimitUnmodified(t *testing.T) {
	const limit = 3
	overFetched := make([]domain.Contact, limit+1)
	stub := &stubService{listContacts: func(ctx context.Context, supplierID int64, criteria domain.ContactListCriteria) ([]domain.Contact, error) {
		if criteria.Limit != limit {
			t.Fatalf("internal ListContacts called with Limit = %d, want %d", criteria.Limit, limit)
		}
		return overFetched, nil
	}}
	adapter := NewAdapter(stub)

	page, err := adapter.ListContacts(context.Background(), public.ContactQuery{SupplierID: 1, Limit: limit})
	if err != nil {
		t.Fatalf("ListContacts error = %v", err)
	}
	if len(page.Contacts) != limit || !page.HasNext {
		t.Fatalf("ListContacts page = %d entries, HasNext = %v; want %d entries, HasNext = true", len(page.Contacts), page.HasNext, limit)
	}
}

func TestAdapter_ListContacts_ForeignBranchYieldsValidation(t *testing.T) {
	stub := &stubService{listContacts: func(ctx context.Context, supplierID int64, criteria domain.ContactListCriteria) ([]domain.Contact, error) {
		return nil, domain.ErrBranchOwnership
	}}
	adapter := NewAdapter(stub)

	foreignBranch := int64(99)
	_, err := adapter.ListContacts(context.Background(), public.ContactQuery{SupplierID: 1, BranchID: &foreignBranch})
	if !public.IsCode(err, public.Validation) {
		t.Fatalf("ListContacts(foreign branch) error code = %v, want %v", public.Code(err), public.Validation)
	}
}

func TestAdapter_ListBranches_UnknownSupplierYieldsNotFound(t *testing.T) {
	stub := &stubService{listBranches: func(ctx context.Context, supplierID int64, criteria domain.ListCriteria) ([]domain.Branch, error) {
		return nil, domain.ErrSupplierNotFound
	}}
	adapter := NewAdapter(stub)

	_, err := adapter.ListBranches(context.Background(), public.BranchQuery{SupplierID: 999})
	if !public.IsCode(err, public.NotFound) {
		t.Fatalf("ListBranches(unknown supplier) error code = %v, want %v", public.Code(err), public.NotFound)
	}
}

func TestAdapter_GetBranch_UnknownBranchYieldsNotFound_NoSupplierPreCheck(t *testing.T) {
	// GetBranch does not confirm the supplier exists first — it surfaces
	// NOT_FOUND directly from the branch lookup itself, preserving the
	// internal asymmetry rather than normalizing it.
	stub := &stubService{getBranch: func(ctx context.Context, supplierID, branchID int64) (domain.Branch, error) {
		return domain.Branch{}, domain.ErrBranchNotFound
	}}
	adapter := NewAdapter(stub)

	_, err := adapter.GetBranch(context.Background(), public.BranchKey{SupplierID: 999, BranchID: 999})
	if !public.IsCode(err, public.NotFound) {
		t.Fatalf("GetBranch(unknown) error code = %v, want %v", public.Code(err), public.NotFound)
	}
}

func TestAdapter_MapBranch_FieldCompleteness(t *testing.T) {
	got := mapBranch(domain.Branch{ID: 1, SupplierID: 2, Name: "n"})
	domainFields := reflect.VisibleFields(reflect.TypeOf(domain.Branch{}))
	publicVal := reflect.ValueOf(got)
	for _, f := range domainFields {
		if !publicVal.FieldByName(f.Name).IsValid() {
			t.Errorf("domain.Branch field %s has no corresponding public.Branch field", f.Name)
		}
	}
}

func TestActiveFromScope_MapsEachLifecycleScope(t *testing.T) {
	trueVal, falseVal := true, false
	cases := []struct {
		name  string
		scope public.LifecycleScope
		want  *bool
	}{
		{"ACTIVE", public.ScopeActive, &trueVal},
		{"empty behaves as ACTIVE", "", &trueVal},
		{"INACTIVE", public.ScopeInactive, &falseVal},
		{"ALL", public.ScopeAll, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := activeFromScope(c.scope)
			if c.want == nil {
				if got != nil {
					t.Fatalf("activeFromScope(%q) = %v, want nil", c.scope, *got)
				}
				return
			}
			if got == nil || *got != *c.want {
				t.Fatalf("activeFromScope(%q) = %v, want %v", c.scope, got, *c.want)
			}
		})
	}
}

func TestAdapter_SearchSuppliers_PassesScopeThroughToInternalCriteria(t *testing.T) {
	cases := []struct {
		name  string
		scope public.LifecycleScope
		want  *bool
	}{
		{"ACTIVE", public.ScopeActive, boolPtr(true)},
		{"INACTIVE", public.ScopeInactive, boolPtr(false)},
		{"ALL", public.ScopeAll, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var gotActive *bool
			captured := false
			stub := &stubService{searchSuppliers: func(ctx context.Context, criteria domain.SupplierSearch) ([]domain.Supplier, error) {
				captured = true
				gotActive = criteria.Active
				return nil, nil
			}}
			adapter := NewAdapter(stub)

			if _, err := adapter.SearchSuppliers(context.Background(), public.SupplierQuery{Scope: c.scope}); err != nil {
				t.Fatalf("SearchSuppliers error = %v", err)
			}
			if !captured {
				t.Fatal("internal SearchSuppliers was never called")
			}
			if c.want == nil {
				if gotActive != nil {
					t.Fatalf("criteria.Active = %v, want nil for scope %q", *gotActive, c.scope)
				}
				return
			}
			if gotActive == nil || *gotActive != *c.want {
				t.Fatalf("criteria.Active = %v, want %v for scope %q", gotActive, *c.want, c.scope)
			}
		})
	}
}

func boolPtr(b bool) *bool { return &b }

func TestAdapter_MapContact_FieldCompletenessAndBranchIDClone(t *testing.T) {
	branchID := int64(7)
	source := domain.Contact{ID: 1, SupplierID: 2, BranchID: &branchID}
	got := mapContact(source)

	domainFields := reflect.VisibleFields(reflect.TypeOf(domain.Contact{}))
	publicVal := reflect.ValueOf(got)
	for _, f := range domainFields {
		if !publicVal.FieldByName(f.Name).IsValid() {
			t.Errorf("domain.Contact field %s has no corresponding public.Contact field", f.Name)
		}
	}
	if got.BranchID == source.BranchID {
		t.Fatal("mapContact returned the source's own BranchID pointer, expected a clone")
	}
}

func TestAdapter_ImplementsWriteCapabilities(t *testing.T) {
	var _ public.WriteCapabilities = NewAdapter(&stubService{})
}

func TestAdapter_CreateSupplier_MapsAllFieldsAndSetsActor(t *testing.T) {
	req := public.SupplierWriteRequest{
		Actor: "PI", TradeName: "ACME", LegalName: "ACME SA", TaxIdentifier: "TAX1", Website: "acme.test", Notes: "n",
	}
	var gotDetails domain.SupplierDetails
	var gotActor string
	stub := &stubService{createSupplier: func(ctx context.Context, details domain.SupplierDetails) (domain.Supplier, error) {
		gotDetails = details
		gotActor = core.ActorFrom(ctx)
		return domain.Supplier{ID: 1, TradeName: details.TradeName, LegalName: details.LegalName, TaxIdentifier: details.TaxIdentifier, Website: details.Website, Notes: details.Notes, Active: true}, nil
	}}
	adapter := NewAdapter(stub)

	got, err := adapter.CreateSupplier(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateSupplier error = %v", err)
	}
	if gotActor != "PI" {
		t.Fatalf("core.ActorFrom(ctx) = %q, want %q", gotActor, "PI")
	}
	if gotDetails != (domain.SupplierDetails{TradeName: "ACME", LegalName: "ACME SA", TaxIdentifier: "TAX1", Website: "acme.test", Notes: "n"}) {
		t.Fatalf("domain.SupplierDetails mapping mismatch: got %+v", gotDetails)
	}

	// Field-completeness: every domain.Supplier field must be reflected in
	// the public mapping.
	domainFields := reflect.VisibleFields(reflect.TypeOf(domain.Supplier{}))
	publicVal := reflect.ValueOf(got)
	for _, f := range domainFields {
		if !publicVal.FieldByName(f.Name).IsValid() {
			t.Errorf("domain.Supplier field %s has no corresponding public.Supplier field", f.Name)
		}
	}
	if got.TradeName != "ACME" || !got.Active {
		t.Fatalf("CreateSupplier mapping mismatch: got %+v", got)
	}
}

func TestAdapter_CreateSupplier_EmptyContent_ReturnsValidation(t *testing.T) {
	stub := &stubService{createSupplier: func(ctx context.Context, details domain.SupplierDetails) (domain.Supplier, error) {
		return domain.Supplier{}, domain.NewValidationError("supplier", "trade name, legal name, or tax identifier is required")
	}}
	adapter := NewAdapter(stub)

	_, err := adapter.CreateSupplier(context.Background(), public.SupplierWriteRequest{Actor: "PI"})
	if !public.IsCode(err, public.Validation) {
		t.Fatalf("CreateSupplier(empty content) error code = %v, want %v", public.Code(err), public.Validation)
	}
}

func TestAdapter_CreateSupplier_DuplicateTaxIdentifier_ReturnsConflict(t *testing.T) {
	stub := &stubService{createSupplier: func(ctx context.Context, details domain.SupplierDetails) (domain.Supplier, error) {
		return domain.Supplier{}, domain.ErrTaxIdentifierConflict
	}}
	adapter := NewAdapter(stub)

	_, err := adapter.CreateSupplier(context.Background(), public.SupplierWriteRequest{Actor: "PI", TaxIdentifier: "TAX1"})
	if !public.IsCode(err, public.Conflict) {
		t.Fatalf("CreateSupplier(duplicate tax identifier) error code = %v, want %v", public.Code(err), public.Conflict)
	}
}

func TestAdapter_CreateSupplier_RawInternalErrorNeverLeaks_ReturnsInternal(t *testing.T) {
	rawErr := errors.New(`ERROR: duplicate key value violates unique constraint "suppliers_tax_identifier_key" (SQLSTATE 23505)`)
	stub := &stubService{createSupplier: func(ctx context.Context, details domain.SupplierDetails) (domain.Supplier, error) {
		return domain.Supplier{}, rawErr
	}}
	adapter := NewAdapter(stub)

	_, err := adapter.CreateSupplier(context.Background(), public.SupplierWriteRequest{Actor: "PI", TaxIdentifier: "TAX1"})
	if !public.IsCode(err, public.Internal) {
		t.Fatalf("CreateSupplier error code = %v, want %v", public.Code(err), public.Internal)
	}
	if err.Error() != "supplier master operation failed" {
		t.Fatalf("mapError default message = %q, want %q", err.Error(), "supplier master operation failed")
	}
	for _, forbidden := range []string{"SQLSTATE", "23505", "suppliers_tax_identifier_key", "duplicate key"} {
		if strings.Contains(err.Error(), forbidden) {
			t.Errorf("public error message %q leaks driver detail %q", err.Error(), forbidden)
		}
	}
}

func TestAdapter_CreateSupplier_MutatingRequestAfterCall_NoLeak(t *testing.T) {
	var captured domain.SupplierDetails
	stub := &stubService{createSupplier: func(ctx context.Context, details domain.SupplierDetails) (domain.Supplier, error) {
		captured = details
		return domain.Supplier{TradeName: details.TradeName}, nil
	}}
	adapter := NewAdapter(stub)

	req := public.SupplierWriteRequest{Actor: "PI", TradeName: "ACME"}
	if _, err := adapter.CreateSupplier(context.Background(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req.TradeName = "MUTATED"

	if captured.TradeName != "ACME" {
		t.Fatalf("caller mutation after call leaked into the internal call: %+v", captured)
	}
}

func TestAdapter_UpdateSupplier_FieldCompleteness(t *testing.T) {
	req := public.SupplierUpdateRequest{
		Actor: "PI", ID: 1, TradeName: "ACME", LegalName: "ACME SA", TaxIdentifier: "TAX1", Website: "acme.test", Notes: "n",
	}
	var gotID int64
	var gotDetails domain.SupplierDetails
	var gotActor string
	stub := &stubService{updateSupplier: func(ctx context.Context, id int64, details domain.SupplierDetails) (domain.Supplier, error) {
		gotID = id
		gotDetails = details
		gotActor = core.ActorFrom(ctx)
		return domain.Supplier{ID: id, TradeName: details.TradeName, LegalName: details.LegalName, TaxIdentifier: details.TaxIdentifier, Website: details.Website, Notes: details.Notes, Active: true}, nil
	}}
	adapter := NewAdapter(stub)

	got, err := adapter.UpdateSupplier(context.Background(), req)
	if err != nil {
		t.Fatalf("UpdateSupplier error = %v", err)
	}
	if gotActor != "PI" {
		t.Fatalf("core.ActorFrom(ctx) = %q, want %q", gotActor, "PI")
	}
	if gotID != 1 {
		t.Fatalf("id passed to internal call = %d, want 1", gotID)
	}
	if gotDetails != (domain.SupplierDetails{TradeName: "ACME", LegalName: "ACME SA", TaxIdentifier: "TAX1", Website: "acme.test", Notes: "n"}) {
		t.Fatalf("domain.SupplierDetails mapping mismatch: got %+v", gotDetails)
	}

	domainFields := reflect.VisibleFields(reflect.TypeOf(domain.Supplier{}))
	publicVal := reflect.ValueOf(got)
	for _, f := range domainFields {
		if !publicVal.FieldByName(f.Name).IsValid() {
			t.Errorf("domain.Supplier field %s has no corresponding public.Supplier field", f.Name)
		}
	}
	if got.TradeName != "ACME" {
		t.Fatalf("UpdateSupplier mapping mismatch: got %+v", got)
	}
}

func TestAdapter_UpdateSupplier_AllFiveErrorCategoriesReachable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want public.ErrorCode
	}{
		{"empty content", domain.NewValidationError("supplier", "trade name, legal name, or tax identifier is required"), public.Validation},
		{"unknown id", domain.ErrSupplierNotFound, public.NotFound},
		{"duplicate tax identifier", domain.ErrTaxIdentifierConflict, public.Conflict},
		{"unclassified failure", errors.New("boom"), public.Internal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &stubService{updateSupplier: func(ctx context.Context, id int64, details domain.SupplierDetails) (domain.Supplier, error) {
				return domain.Supplier{}, tt.err
			}}
			adapter := NewAdapter(stub)
			_, err := adapter.UpdateSupplier(context.Background(), public.SupplierUpdateRequest{Actor: "PI", ID: 1})
			if !public.IsCode(err, tt.want) {
				t.Fatalf("UpdateSupplier(%s) error code = %v, want %v", tt.name, public.Code(err), tt.want)
			}
		})
	}
	// INVALID_ARGUMENT is a Writer-level shape rejection, proven at the
	// suppliercore package level (TestWriter_UpdateSupplier_RejectsBlankActorOrNonPositiveID);
	// the bridge is never reached for a shape-invalid request.
}

func TestAdapter_UpdateSupplier_InactiveSupplierStaysInactive(t *testing.T) {
	stub := &stubService{updateSupplier: func(ctx context.Context, id int64, details domain.SupplierDetails) (domain.Supplier, error) {
		return domain.Supplier{ID: id, TradeName: details.TradeName, Active: false}, nil
	}}
	adapter := NewAdapter(stub)

	got, err := adapter.UpdateSupplier(context.Background(), public.SupplierUpdateRequest{Actor: "PI", ID: 1, TradeName: "ACME"})
	if err != nil {
		t.Fatalf("UpdateSupplier error = %v", err)
	}
	if got.Active {
		t.Fatalf("UpdateSupplier on an inactive supplier returned Active = true, want false (Update must never reactivate)")
	}
}

func TestAdapter_UpdateSupplier_ActiveSupplierNeverFlipped(t *testing.T) {
	stub := &stubService{updateSupplier: func(ctx context.Context, id int64, details domain.SupplierDetails) (domain.Supplier, error) {
		return domain.Supplier{ID: id, TradeName: details.TradeName, Active: true}, nil
	}}
	adapter := NewAdapter(stub)

	got, err := adapter.UpdateSupplier(context.Background(), public.SupplierUpdateRequest{Actor: "PI", ID: 1, TradeName: "ACME"})
	if err != nil {
		t.Fatalf("UpdateSupplier error = %v", err)
	}
	if !got.Active {
		t.Fatalf("UpdateSupplier on an active supplier returned Active = false, want true")
	}
}

func TestAdapter_UpdateSupplier_MutatingRequestAfterCall_NoLeak(t *testing.T) {
	var captured domain.SupplierDetails
	stub := &stubService{updateSupplier: func(ctx context.Context, id int64, details domain.SupplierDetails) (domain.Supplier, error) {
		captured = details
		return domain.Supplier{TradeName: details.TradeName}, nil
	}}
	adapter := NewAdapter(stub)

	req := public.SupplierUpdateRequest{Actor: "PI", ID: 1, TradeName: "ACME"}
	if _, err := adapter.UpdateSupplier(context.Background(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req.TradeName = "MUTATED"

	if captured.TradeName != "ACME" {
		t.Fatalf("caller mutation after call leaked into the internal call: %+v", captured)
	}
}

func TestAdapter_GetSupplierByTaxIdentifier_MapsSupplier(t *testing.T) {
	var gotTaxID string
	adapter := NewAdapter(&stubService{getByTaxID: func(_ context.Context, taxID string) (domain.Supplier, error) {
		gotTaxID = taxID
		return domain.Supplier{ID: 9, LegalName: "ACME SA", TaxIdentifier: "ACM010101AA1", Active: false}, nil
	}})

	got, err := adapter.GetSupplierByTaxIdentifier(context.Background(), "acm010101aa1")
	if err != nil {
		t.Fatalf("GetSupplierByTaxIdentifier error = %v", err)
	}
	if gotTaxID != "acm010101aa1" {
		t.Errorf("service received %q, want the value untouched", gotTaxID)
	}
	if got.ID != 9 || got.LegalName != "ACME SA" || got.TaxIdentifier != "ACM010101AA1" || got.Active {
		t.Errorf("mapped supplier = %#v", got)
	}
}

func TestAdapter_GetSupplierByTaxIdentifier_ClassifiesErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want public.ErrorCode
	}{
		{"not found", domain.ErrSupplierNotFound, public.NotFound},
		{"validation", domain.NewValidationError("tax_identifier", "must not be blank"), public.Validation},
		{"raw error never leaks", errors.New("pq: connection refused"), public.Internal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewAdapter(&stubService{getByTaxID: func(context.Context, string) (domain.Supplier, error) { return domain.Supplier{}, tt.err }})
			_, err := adapter.GetSupplierByTaxIdentifier(context.Background(), "ABC010101AA1")
			if public.Code(err) != tt.want {
				t.Fatalf("code = %v, want %v", public.Code(err), tt.want)
			}
			if strings.Contains(err.Error(), "connection refused") {
				t.Fatalf("raw infrastructure error leaked: %v", err)
			}
		})
	}
}
