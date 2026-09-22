package suppliercore_test

import (
	"context"
	"testing"

	"github.com/GARFEX33/garfex-backend/suppliercore"
)

// fakeCapabilities is a minimal external ReadCapabilities implementation,
// proving suppliercore.Reader can be constructed and used from a package
// that imports no internal/... path.
type fakeCapabilities struct{}

func (fakeCapabilities) GetSupplier(ctx context.Context, id int64) (suppliercore.Supplier, error) {
	return suppliercore.Supplier{ID: id, TradeName: "External Supplier"}, nil
}
func (fakeCapabilities) GetSupplierByTaxIdentifier(ctx context.Context, taxID string) (suppliercore.Supplier, error) {
	return suppliercore.Supplier{ID: 5, TaxIdentifier: taxID}, nil
}
func (fakeCapabilities) SearchSuppliers(ctx context.Context, q suppliercore.SupplierQuery) (suppliercore.SupplierPage, error) {
	return suppliercore.SupplierPage{Suppliers: []suppliercore.Supplier{{ID: 1}}}, nil
}
func (fakeCapabilities) ListBranches(ctx context.Context, q suppliercore.BranchQuery) (suppliercore.BranchPage, error) {
	return suppliercore.BranchPage{Branches: []suppliercore.Branch{{ID: 1, SupplierID: q.SupplierID}}}, nil
}
func (fakeCapabilities) GetBranch(ctx context.Context, key suppliercore.BranchKey) (suppliercore.Branch, error) {
	return suppliercore.Branch{ID: key.BranchID, SupplierID: key.SupplierID}, nil
}
func (fakeCapabilities) ListContacts(ctx context.Context, q suppliercore.ContactQuery) (suppliercore.ContactPage, error) {
	return suppliercore.ContactPage{Contacts: []suppliercore.Contact{{ID: 1, SupplierID: q.SupplierID}}}, nil
}
func (fakeCapabilities) GetContact(ctx context.Context, key suppliercore.ContactKey) (suppliercore.Contact, error) {
	return suppliercore.Contact{ID: key.ContactID, SupplierID: key.SupplierID}, nil
}

func TestExternalConsumer_ReadsAllThreeEntities(t *testing.T) {
	reader, err := suppliercore.NewReadOnly(fakeCapabilities{})
	if err != nil {
		t.Fatalf("NewReadOnly error = %v", err)
	}

	ctx := context.Background()

	if _, err := reader.GetSupplier(ctx, 1); err != nil {
		t.Fatalf("GetSupplier error = %v", err)
	}
	if got, err := reader.GetSupplierByTaxIdentifier(ctx, "ABC010101AA1"); err != nil || got.ID != 5 {
		t.Fatalf("GetSupplierByTaxIdentifier = %#v, %v", got, err)
	}
	if _, err := reader.ListBranches(ctx, suppliercore.BranchQuery{SupplierID: 1}); err != nil {
		t.Fatalf("ListBranches error = %v", err)
	}
	if _, err := reader.ListContacts(ctx, suppliercore.ContactQuery{SupplierID: 1}); err != nil {
		t.Fatalf("ListContacts error = %v", err)
	}
}

// fakeWriteCapabilities is a minimal external WriteCapabilities
// implementation, proving suppliercore.Writer can be constructed and used
// from a package that imports no internal/... path.
type fakeWriteCapabilities struct{}

func (fakeWriteCapabilities) CreateSupplier(ctx context.Context, req suppliercore.SupplierWriteRequest) (suppliercore.Supplier, error) {
	return suppliercore.Supplier{ID: 1, TradeName: req.TradeName, Active: true}, nil
}

func (fakeWriteCapabilities) UpdateSupplier(ctx context.Context, req suppliercore.SupplierUpdateRequest) (suppliercore.Supplier, error) {
	return suppliercore.Supplier{ID: req.ID, TradeName: req.TradeName, Active: true}, nil
}

func TestExternalConsumer_CreatesSupplier(t *testing.T) {
	writer, err := suppliercore.NewWriter(fakeWriteCapabilities{})
	if err != nil {
		t.Fatalf("NewWriter error = %v", err)
	}

	got, err := writer.CreateSupplier(context.Background(), suppliercore.SupplierWriteRequest{Actor: "PI", TradeName: "ACME"})
	if err != nil {
		t.Fatalf("CreateSupplier error = %v", err)
	}
	if got.TradeName != "ACME" {
		t.Fatalf("CreateSupplier result = %+v, want TradeName ACME", got)
	}
}

func TestExternalConsumer_UpdatesSupplier(t *testing.T) {
	writer, err := suppliercore.NewWriter(fakeWriteCapabilities{})
	if err != nil {
		t.Fatalf("NewWriter error = %v", err)
	}

	got, err := writer.UpdateSupplier(context.Background(), suppliercore.SupplierUpdateRequest{Actor: "PI", ID: 1, TradeName: "ACME UPDATED"})
	if err != nil {
		t.Fatalf("UpdateSupplier error = %v", err)
	}
	if got.TradeName != "ACME UPDATED" {
		t.Fatalf("UpdateSupplier result = %+v, want TradeName ACME UPDATED", got)
	}
}
