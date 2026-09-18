package suppliercore

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type stubCapabilities struct {
	called bool

	getSupplier     func(ctx context.Context, id int64) (Supplier, error)
	getByTaxID      func(ctx context.Context, taxID string) (Supplier, error)
	searchSuppliers func(ctx context.Context, q SupplierQuery) (SupplierPage, error)
	listBranches    func(ctx context.Context, q BranchQuery) (BranchPage, error)
	getBranch       func(ctx context.Context, key BranchKey) (Branch, error)
	listContacts    func(ctx context.Context, q ContactQuery) (ContactPage, error)
	getContact      func(ctx context.Context, key ContactKey) (Contact, error)
}

func (s *stubCapabilities) GetSupplier(ctx context.Context, id int64) (Supplier, error) {
	s.called = true
	return s.getSupplier(ctx, id)
}
func (s *stubCapabilities) GetSupplierByTaxIdentifier(ctx context.Context, taxID string) (Supplier, error) {
	s.called = true
	return s.getByTaxID(ctx, taxID)
}
func (s *stubCapabilities) SearchSuppliers(ctx context.Context, q SupplierQuery) (SupplierPage, error) {
	s.called = true
	return s.searchSuppliers(ctx, q)
}
func (s *stubCapabilities) ListBranches(ctx context.Context, q BranchQuery) (BranchPage, error) {
	s.called = true
	return s.listBranches(ctx, q)
}
func (s *stubCapabilities) GetBranch(ctx context.Context, key BranchKey) (Branch, error) {
	s.called = true
	return s.getBranch(ctx, key)
}
func (s *stubCapabilities) ListContacts(ctx context.Context, q ContactQuery) (ContactPage, error) {
	s.called = true
	return s.listContacts(ctx, q)
}
func (s *stubCapabilities) GetContact(ctx context.Context, key ContactKey) (Contact, error) {
	s.called = true
	return s.getContact(ctx, key)
}

func TestNewReadOnly_RejectsNilCapability(t *testing.T) {
	r, err := NewReadOnly(nil)
	if r != nil {
		t.Fatalf("NewReadOnly(nil) reader = %v, want nil", r)
	}
	if !IsCode(err, InvalidArgument) {
		t.Fatalf("NewReadOnly(nil) error code = %v, want %v", Code(err), InvalidArgument)
	}
}

func TestReader_GetSupplier_RejectsNonPositiveID(t *testing.T) {
	stub := &stubCapabilities{}
	reader, _ := NewReadOnly(stub)

	for _, id := range []int64{0, -1} {
		_, err := reader.GetSupplier(context.Background(), id)
		if !IsCode(err, InvalidArgument) {
			t.Errorf("GetSupplier(%d) code = %v, want %v", id, Code(err), InvalidArgument)
		}
		if stub.called {
			t.Errorf("GetSupplier(%d) reached the capability, should have been rejected at the boundary", id)
		}
	}
}

func TestReader_GetBranch_RejectsNonPositiveIDs(t *testing.T) {
	stub := &stubCapabilities{}
	reader, _ := NewReadOnly(stub)

	cases := []BranchKey{{SupplierID: 0, BranchID: 1}, {SupplierID: 1, BranchID: 0}, {SupplierID: -1, BranchID: -1}}
	for _, key := range cases {
		_, err := reader.GetBranch(context.Background(), key)
		if !IsCode(err, InvalidArgument) {
			t.Errorf("GetBranch(%+v) code = %v, want %v", key, Code(err), InvalidArgument)
		}
		if stub.called {
			t.Errorf("GetBranch(%+v) reached the capability, should have been rejected", key)
		}
	}
}

func TestReader_GetContact_RejectsNonPositiveIDs(t *testing.T) {
	stub := &stubCapabilities{}
	reader, _ := NewReadOnly(stub)

	cases := []ContactKey{{SupplierID: 0, ContactID: 1}, {SupplierID: 1, ContactID: 0}}
	for _, key := range cases {
		_, err := reader.GetContact(context.Background(), key)
		if !IsCode(err, InvalidArgument) {
			t.Errorf("GetContact(%+v) code = %v, want %v", key, Code(err), InvalidArgument)
		}
		if stub.called {
			t.Errorf("GetContact(%+v) reached the capability, should have been rejected", key)
		}
	}
}

func TestReader_SearchSuppliers_RejectsUnknownScope(t *testing.T) {
	stub := &stubCapabilities{}
	reader, _ := NewReadOnly(stub)

	_, err := reader.SearchSuppliers(context.Background(), SupplierQuery{Scope: "BOGUS"})
	if !IsCode(err, InvalidArgument) {
		t.Fatalf("SearchSuppliers(bogus scope) code = %v, want %v", Code(err), InvalidArgument)
	}
	if stub.called {
		t.Fatal("SearchSuppliers(bogus scope) reached the capability, should have been rejected")
	}
}

func TestReader_ListBranches_RejectsNonPositiveSupplierID(t *testing.T) {
	stub := &stubCapabilities{}
	reader, _ := NewReadOnly(stub)

	_, err := reader.ListBranches(context.Background(), BranchQuery{SupplierID: 0})
	if !IsCode(err, InvalidArgument) {
		t.Fatalf("ListBranches(SupplierID: 0) code = %v, want %v", Code(err), InvalidArgument)
	}
	if stub.called {
		t.Fatal("ListBranches(SupplierID: 0) reached the capability, should have been rejected")
	}
}

func TestReader_ListContacts_RejectsNonPositiveBranchIDFilter(t *testing.T) {
	stub := &stubCapabilities{}
	reader, _ := NewReadOnly(stub)

	zero := int64(0)
	_, err := reader.ListContacts(context.Background(), ContactQuery{SupplierID: 1, BranchID: &zero})
	if !IsCode(err, InvalidArgument) {
		t.Fatalf("ListContacts(BranchID: 0) code = %v, want %v", Code(err), InvalidArgument)
	}
	if stub.called {
		t.Fatal("ListContacts(BranchID: 0) reached the capability, should have been rejected")
	}
}

func TestReader_ListContacts_AllowsNilBranchIDFilter(t *testing.T) {
	stub := &stubCapabilities{
		listContacts: func(ctx context.Context, q ContactQuery) (ContactPage, error) {
			return ContactPage{}, nil
		},
	}
	reader, _ := NewReadOnly(stub)

	if _, err := reader.ListContacts(context.Background(), ContactQuery{SupplierID: 1, BranchID: nil}); err != nil {
		t.Fatalf("ListContacts(nil BranchID) error = %v, want nil", err)
	}
	if !stub.called {
		t.Fatal("ListContacts(nil BranchID) should have reached the capability")
	}
}

func TestReader_GetContact_ClonesReturnedBranchIDPointer(t *testing.T) {
	branchID := int64(5)
	stub := &stubCapabilities{
		getContact: func(ctx context.Context, key ContactKey) (Contact, error) {
			return Contact{ID: 1, BranchID: &branchID}, nil
		},
	}
	reader, _ := NewReadOnly(stub)

	got, err := reader.GetContact(context.Background(), ContactKey{SupplierID: 1, ContactID: 1})
	if err != nil {
		t.Fatalf("GetContact error = %v", err)
	}
	if got.BranchID == &branchID {
		t.Fatal("GetContact returned the capability's own BranchID pointer, expected a clone")
	}
}

func TestReader_PropagatesCapabilityError(t *testing.T) {
	wantErr := NewError(NotFound, "supplier not found")
	stub := &stubCapabilities{
		getSupplier: func(ctx context.Context, id int64) (Supplier, error) {
			return Supplier{}, wantErr
		},
	}
	reader, _ := NewReadOnly(stub)

	_, err := reader.GetSupplier(context.Background(), 1)
	if !errors.Is(err, error(wantErr)) && !IsCode(err, NotFound) {
		t.Fatalf("GetSupplier propagated error code = %v, want %v", Code(err), NotFound)
	}
}

func TestReader_NoUngraduatedMethodExported(t *testing.T) {
	allowed := map[string]bool{
		"GetSupplier": true, "GetSupplierByTaxIdentifier": true, "SearchSuppliers": true,
		"ListBranches": true, "GetBranch": true,
		"ListContacts": true, "GetContact": true,
	}

	capType := reflect.TypeOf((*ReadCapabilities)(nil)).Elem()
	if capType.NumMethod() != len(allowed) {
		t.Fatalf("ReadCapabilities has %d methods, want %d", capType.NumMethod(), len(allowed))
	}
	for i := 0; i < capType.NumMethod(); i++ {
		name := capType.Method(i).Name
		if !allowed[name] {
			t.Errorf("ReadCapabilities declares ungraduated method %s", name)
		}
	}

	readerType := reflect.TypeOf(&Reader{})
	for i := 0; i < readerType.NumMethod(); i++ {
		name := readerType.Method(i).Name
		if !allowed[name] {
			t.Errorf("Reader exports ungraduated method %s", name)
		}
	}
}

func TestReader_GetSupplierByTaxIdentifier_RejectsBlank(t *testing.T) {
	for _, taxID := range []string{"", "   ", "\t\n"} {
		stub := &stubCapabilities{}
		reader, _ := NewReadOnly(stub)
		_, err := reader.GetSupplierByTaxIdentifier(context.Background(), taxID)
		if !IsCode(err, InvalidArgument) {
			t.Errorf("GetSupplierByTaxIdentifier(%q) code = %v, want %v", taxID, Code(err), InvalidArgument)
		}
		if stub.called {
			t.Errorf("GetSupplierByTaxIdentifier(%q) reached the capability, should have been rejected at the boundary", taxID)
		}
	}
}

func TestReader_GetSupplierByTaxIdentifier_DelegatesAndClones(t *testing.T) {
	var gotTaxID string
	stub := &stubCapabilities{getByTaxID: func(_ context.Context, taxID string) (Supplier, error) {
		gotTaxID = taxID
		return Supplier{ID: 7, TaxIdentifier: "ABC010101AA1", Active: false}, nil
	}}
	reader, _ := NewReadOnly(stub)

	got, err := reader.GetSupplierByTaxIdentifier(context.Background(), "abc010101aa1")
	if err != nil {
		t.Fatalf("GetSupplierByTaxIdentifier error = %v", err)
	}
	if gotTaxID != "abc010101aa1" {
		t.Errorf("capability received %q, want the caller value untouched", gotTaxID)
	}
	if got.ID != 7 || got.Active {
		t.Errorf("got %#v, want the inactive supplier 7 returned as is", got)
	}
}

func TestReader_GetSupplierByTaxIdentifier_PropagatesCapabilityError(t *testing.T) {
	want := NewError(NotFound, "supplier not found")
	stub := &stubCapabilities{getByTaxID: func(context.Context, string) (Supplier, error) { return Supplier{}, want }}
	reader, _ := NewReadOnly(stub)

	_, err := reader.GetSupplierByTaxIdentifier(context.Background(), "ABC010101AA1")
	if !errors.Is(err, want) && !IsCode(err, NotFound) {
		t.Fatalf("error = %v, want NotFound", err)
	}
}
