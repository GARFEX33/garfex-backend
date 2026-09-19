package purchasecore_test

import (
	"context"
	"testing"

	"github.com/GARFEX33/garfex-costos-unitarios/purchasecore"
)

// fakeReadCapabilities is a minimal external ReadCapabilities
// implementation, proving purchasecore.Reader can be constructed and used
// from a package that imports no internal/... path.
type fakeReadCapabilities struct{}

func (fakeReadCapabilities) GetPurchase(ctx context.Context, id int64) (purchasecore.Purchase, error) {
	return purchasecore.Purchase{ID: id, CFDIUUID: "ABCDEF12-3456-7890-ABCD-EF1234567890"}, nil
}
func (fakeReadCapabilities) GetPurchaseByUUID(ctx context.Context, uuid string) (purchasecore.Purchase, error) {
	return purchasecore.Purchase{ID: 1, CFDIUUID: uuid}, nil
}
func (fakeReadCapabilities) ListPurchaseLines(ctx context.Context, purchaseID int64) ([]purchasecore.PurchaseLine, error) {
	return []purchasecore.PurchaseLine{{ID: 1, PurchaseID: purchaseID}}, nil
}
func (fakeReadCapabilities) ListPurchasesBySupplier(ctx context.Context, supplierID int64, q purchasecore.ListCriteria) (purchasecore.PurchasePage, error) {
	return purchasecore.PurchasePage{Purchases: []purchasecore.Purchase{{ID: 1, SupplierID: supplierID}}}, nil
}
func (fakeReadCapabilities) GetSupplierProduct(ctx context.Context, id int64) (purchasecore.SupplierProduct, error) {
	return purchasecore.SupplierProduct{ID: id}, nil
}
func (fakeReadCapabilities) FindSupplierProduct(ctx context.Context, supplierID int64, sku string) (purchasecore.SupplierProduct, error) {
	return purchasecore.SupplierProduct{ID: 1, SupplierID: supplierID, SupplierSKU: sku}, nil
}
func (fakeReadCapabilities) ListSupplierProducts(ctx context.Context, supplierID int64, q purchasecore.ListCriteria) (purchasecore.SupplierProductPage, error) {
	return purchasecore.SupplierProductPage{Products: []purchasecore.SupplierProduct{{ID: 1, SupplierID: supplierID}}}, nil
}
func (fakeReadCapabilities) ListPurchaseLinesByResource(ctx context.Context, resourceID int64, q purchasecore.ListCriteria) (purchasecore.PurchaseLineHistoryPage, error) {
	return purchasecore.PurchaseLineHistoryPage{History: []purchasecore.PurchaseLineHistory{{PurchaseID: 1}}}, nil
}

func TestExternalConsumer_ReadsEveryEntity(t *testing.T) {
	reader, err := purchasecore.NewReadOnly(fakeReadCapabilities{})
	if err != nil {
		t.Fatalf("NewReadOnly error = %v", err)
	}
	ctx := context.Background()

	if _, err := reader.GetPurchase(ctx, 1); err != nil {
		t.Fatalf("GetPurchase error = %v", err)
	}
	if got, err := reader.GetPurchaseByUUID(ctx, "abc"); err != nil || got.CFDIUUID != "abc" {
		t.Fatalf("GetPurchaseByUUID = %#v, %v", got, err)
	}
	if _, err := reader.ListPurchaseLines(ctx, 1); err != nil {
		t.Fatalf("ListPurchaseLines error = %v", err)
	}
	if _, err := reader.ListPurchasesBySupplier(ctx, 1, purchasecore.ListCriteria{}); err != nil {
		t.Fatalf("ListPurchasesBySupplier error = %v", err)
	}
	if _, err := reader.GetSupplierProduct(ctx, 1); err != nil {
		t.Fatalf("GetSupplierProduct error = %v", err)
	}
	if _, err := reader.FindSupplierProduct(ctx, 1, "SKU-1"); err != nil {
		t.Fatalf("FindSupplierProduct error = %v", err)
	}
	if _, err := reader.ListSupplierProducts(ctx, 1, purchasecore.ListCriteria{}); err != nil {
		t.Fatalf("ListSupplierProducts error = %v", err)
	}
	if _, err := reader.ListPurchaseLinesByResource(ctx, 1, purchasecore.ListCriteria{}); err != nil {
		t.Fatalf("ListPurchaseLinesByResource error = %v", err)
	}
}

func TestReader_RejectsInvalidShape(t *testing.T) {
	reader, err := purchasecore.NewReadOnly(fakeReadCapabilities{})
	if err != nil {
		t.Fatalf("NewReadOnly error = %v", err)
	}
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{"GetPurchase non-positive id", func() error { _, err := reader.GetPurchase(ctx, 0); return err }},
		{"GetPurchaseByUUID blank uuid", func() error { _, err := reader.GetPurchaseByUUID(ctx, "  "); return err }},
		{"ListPurchaseLines non-positive id", func() error { _, err := reader.ListPurchaseLines(ctx, 0); return err }},
		{"ListPurchasesBySupplier non-positive id", func() error {
			_, err := reader.ListPurchasesBySupplier(ctx, 0, purchasecore.ListCriteria{})
			return err
		}},
		{"FindSupplierProduct blank sku", func() error { _, err := reader.FindSupplierProduct(ctx, 1, " "); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if !purchasecore.IsCode(err, purchasecore.InvalidArgument) {
				t.Fatalf("error = %v, want INVALID_ARGUMENT", err)
			}
		})
	}
}

func TestReadOnly_RejectsNilCapabilities(t *testing.T) {
	if _, err := purchasecore.NewReadOnly(nil); !purchasecore.IsCode(err, purchasecore.InvalidArgument) {
		t.Fatalf("error = %v, want INVALID_ARGUMENT", err)
	}
}

// fakeWriteCapabilities is a minimal external WriteCapabilities
// implementation, proving purchasecore.Writer can be constructed and used
// from a package that imports no internal/... path.
type fakeWriteCapabilities struct{}

func (fakeWriteCapabilities) ImportPurchase(ctx context.Context, req purchasecore.ImportRequest) (purchasecore.ImportResult, error) {
	return purchasecore.ImportResult{Purchase: purchasecore.Purchase{ID: 1, XML: purchasecore.XMLDocument{Content: req.XML}}}, nil
}
func (fakeWriteCapabilities) LinkSupplierProductToResource(ctx context.Context, req purchasecore.LinkSupplierProductRequest) (purchasecore.SupplierProduct, error) {
	return purchasecore.SupplierProduct{ID: req.SupplierProductID, ResourceID: &req.ResourceID}, nil
}
func (fakeWriteCapabilities) UnlinkSupplierProduct(ctx context.Context, req purchasecore.UnlinkSupplierProductRequest) (purchasecore.SupplierProduct, error) {
	return purchasecore.SupplierProduct{ID: req.SupplierProductID}, nil
}
func (fakeWriteCapabilities) SetPurchaseLineLinkStatus(ctx context.Context, req purchasecore.SetPurchaseLineLinkStatusRequest) (purchasecore.PurchaseLine, error) {
	return purchasecore.PurchaseLine{ID: req.PurchaseLineID, LinkStatus: req.Status}, nil
}

func TestExternalConsumer_ImportsAndLinks(t *testing.T) {
	writer, err := purchasecore.NewWriter(fakeWriteCapabilities{})
	if err != nil {
		t.Fatalf("NewWriter error = %v", err)
	}
	ctx := context.Background()

	result, err := writer.ImportPurchase(ctx, purchasecore.ImportRequest{Actor: "PI", XML: []byte("<xml/>")})
	if err != nil || result.Purchase.ID != 1 {
		t.Fatalf("ImportPurchase = %#v, %v", result, err)
	}
	if _, err := writer.LinkSupplierProductToResource(ctx, purchasecore.LinkSupplierProductRequest{Actor: "PI", SupplierProductID: 1, ResourceID: 2}); err != nil {
		t.Fatalf("LinkSupplierProductToResource error = %v", err)
	}
	if _, err := writer.UnlinkSupplierProduct(ctx, purchasecore.UnlinkSupplierProductRequest{Actor: "PI", SupplierProductID: 1}); err != nil {
		t.Fatalf("UnlinkSupplierProduct error = %v", err)
	}
	got, err := writer.SetPurchaseLineLinkStatus(ctx, purchasecore.SetPurchaseLineLinkStatusRequest{Actor: "PI", PurchaseLineID: 1, Status: purchasecore.LinkNotApplicable})
	if err != nil || got.LinkStatus != purchasecore.LinkNotApplicable {
		t.Fatalf("SetPurchaseLineLinkStatus = %#v, %v", got, err)
	}
}

func TestWriter_RejectsInvalidShape(t *testing.T) {
	writer, err := purchasecore.NewWriter(fakeWriteCapabilities{})
	if err != nil {
		t.Fatalf("NewWriter error = %v", err)
	}
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{"ImportPurchase blank actor", func() error {
			_, err := writer.ImportPurchase(ctx, purchasecore.ImportRequest{XML: []byte("<xml/>")})
			return err
		}},
		{"ImportPurchase empty xml", func() error {
			_, err := writer.ImportPurchase(ctx, purchasecore.ImportRequest{Actor: "PI"})
			return err
		}},
		{"LinkSupplierProductToResource non-positive resource id", func() error {
			_, err := writer.LinkSupplierProductToResource(ctx, purchasecore.LinkSupplierProductRequest{Actor: "PI", SupplierProductID: 1})
			return err
		}},
		{"SetPurchaseLineLinkStatus invalid status", func() error {
			_, err := writer.SetPurchaseLineLinkStatus(ctx, purchasecore.SetPurchaseLineLinkStatusRequest{Actor: "PI", PurchaseLineID: 1, Status: "BOGUS"})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if !purchasecore.IsCode(err, purchasecore.InvalidArgument) {
				t.Fatalf("error = %v, want INVALID_ARGUMENT", err)
			}
		})
	}
}

func TestNewWriter_RejectsNilCapabilities(t *testing.T) {
	if _, err := purchasecore.NewWriter(nil); !purchasecore.IsCode(err, purchasecore.InvalidArgument) {
		t.Fatalf("error = %v, want INVALID_ARGUMENT", err)
	}
}
