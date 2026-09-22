package purchasecore_test

import (
	"context"
	"testing"
	"time"

	"github.com/GARFEX33/garfex-backend/purchasecore"
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
func (fakeReadCapabilities) ListPurchaseLinesWorkbench(ctx context.Context, q purchasecore.PurchaseLineQuery) (purchasecore.PurchaseLinePage, error) {
	return purchasecore.PurchaseLinePage{Rows: []purchasecore.PurchaseLineRow{{LineID: 1}}}, nil
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
func (fakeReadCapabilities) ListMappingAudit(ctx context.Context, supplierProductID int64, q purchasecore.ListCriteria) (purchasecore.MappingAuditPage, error) {
	return purchasecore.MappingAuditPage{Entries: []purchasecore.MappingAuditEntry{{SupplierProductID: supplierProductID}}}, nil
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
	if _, err := reader.ListPurchaseLinesWorkbench(ctx, purchasecore.PurchaseLineQuery{}); err != nil {
		t.Fatalf("ListPurchaseLinesWorkbench error = %v", err)
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
	if _, err := reader.ListMappingAudit(ctx, 1, purchasecore.ListCriteria{}); err != nil {
		t.Fatalf("ListMappingAudit error = %v", err)
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
		{"PurchaseLineWorkbench negative limit", func() error {
			_, err := reader.ListPurchaseLinesWorkbench(ctx, purchasecore.PurchaseLineQuery{Limit: -1})
			return err
		}},
		{"PurchaseLineWorkbench invalid status", func() error {
			_, err := reader.ListPurchaseLinesWorkbench(ctx, purchasecore.PurchaseLineQuery{EffectiveStatus: "UNKNOWN"})
			return err
		}},
		{"PurchaseLineWorkbench invalid date window", func() error {
			from := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
			to := from.Add(-time.Hour)
			_, err := reader.ListPurchaseLinesWorkbench(ctx, purchasecore.PurchaseLineQuery{DateFrom: &from, DateTo: &to})
			return err
		}},
		{"ListPurchasesBySupplier non-positive id", func() error {
			_, err := reader.ListPurchasesBySupplier(ctx, 0, purchasecore.ListCriteria{})
			return err
		}},
		{"FindSupplierProduct blank sku", func() error { _, err := reader.FindSupplierProduct(ctx, 1, " "); return err }},
		{"ListPurchaseLinesWorkbench limit above 50", func() error {
			_, err := reader.ListPurchaseLinesWorkbench(ctx, purchasecore.PurchaseLineQuery{Limit: 51})
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

func TestReadOnly_RejectsNilCapabilities(t *testing.T) {
	if _, err := purchasecore.NewReadOnly(nil); !purchasecore.IsCode(err, purchasecore.InvalidArgument) {
		t.Fatalf("error = %v, want INVALID_ARGUMENT", err)
	}
}

// fakeWriteCapabilities is a minimal external WriteCapabilities implementation.
type fakeWriteCapabilities struct{}

func (fakeWriteCapabilities) ImportPurchase(ctx context.Context, req purchasecore.ImportRequest) (purchasecore.ImportResult, error) {
	return purchasecore.ImportResult{Purchase: purchasecore.Purchase{ID: 1, XML: purchasecore.XMLDocument{Content: req.XML}}}, nil
}
func (fakeWriteCapabilities) ConfirmMapping(ctx context.Context, req purchasecore.ConfirmMappingRequest) (purchasecore.SupplierProduct, error) {
	return purchasecore.SupplierProduct{ID: req.SupplierProductID, CurrentMapping: purchasecore.SupplierProductMapping{ResourceID: &req.ResourceID}}, nil
}
func (fakeWriteCapabilities) CorrectMapping(ctx context.Context, req purchasecore.CorrectMappingRequest) (purchasecore.SupplierProduct, error) {
	return purchasecore.SupplierProduct{ID: req.SupplierProductID, CurrentMapping: purchasecore.SupplierProductMapping{ResourceID: &req.ResourceID}}, nil
}
func (fakeWriteCapabilities) ExceptionalUnlink(ctx context.Context, req purchasecore.ExceptionalUnlinkRequest) (purchasecore.SupplierProduct, error) {
	return purchasecore.SupplierProduct{ID: req.SupplierProductID}, nil
}
func (fakeWriteCapabilities) ReportIdentityConflict(ctx context.Context, req purchasecore.ReportIdentityConflictRequest) (purchasecore.SupplierProduct, error) {
	return purchasecore.SupplierProduct{ID: req.SupplierProductID}, nil
}
func (fakeWriteCapabilities) ResolveIdentityConflict(ctx context.Context, req purchasecore.ResolveIdentityConflictRequest) (purchasecore.SupplierProduct, error) {
	return purchasecore.SupplierProduct{ID: req.SupplierProductID, CurrentMapping: purchasecore.SupplierProductMapping{ResourceID: &req.ResourceID}}, nil
}
func (fakeWriteCapabilities) ResolvePurchaseLine(ctx context.Context, req purchasecore.ResolvePurchaseLineRequest) (purchasecore.ResolvePurchaseLineResult, error) {
	productID := int64(3)
	return purchasecore.ResolvePurchaseLineResult{
		Line:                          purchasecore.PurchaseLine{ID: req.LineID, SupplierProductID: &productID, ResolutionRevision: req.ExpectedResolutionRevision, EffectiveStatus: purchasecore.LinkLinked},
		SupplierProduct:               purchasecore.SupplierProduct{ID: productID, CurrentMapping: purchasecore.SupplierProductMapping{ResourceID: &req.ResourceID}},
		CommercialIdentityDisposition: purchasecore.CommercialIdentityCreated,
	}, nil
}
func (fakeWriteCapabilities) SetResolutionOverride(ctx context.Context, req purchasecore.SetResolutionOverrideRequest) (purchasecore.PurchaseLine, error) {
	return purchasecore.PurchaseLine{ID: req.LineID, ResolutionRevision: req.ExpectedRevision + 1, ResolutionOverride: req.Override, EffectiveStatus: req.Override}, nil
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
	decision := purchasecore.MappingDecisionMetadata{Actor: "PI", Origin: purchasecore.MappingOriginManual, At: time.Now()}
	if _, err := writer.ConfirmMapping(ctx, purchasecore.ConfirmMappingRequest{SupplierProductID: 1, ResourceID: 2, Decision: decision}); err != nil {
		t.Fatalf("ConfirmMapping error = %v", err)
	}
	if _, err := writer.ExceptionalUnlink(ctx, purchasecore.ExceptionalUnlinkRequest{SupplierProductID: 1, ExpectedCurrentResourceID: 2, Decision: decision}); err != nil {
		t.Fatalf("ExceptionalUnlink error = %v", err)
	}
	resolved, err := writer.ResolvePurchaseLine(ctx, purchasecore.ResolvePurchaseLineRequest{
		LineID: 9, ResourceID: 2, CommercialSupplierSKU: "SKU-POSTERIOR", Actor: "PI",
	})
	if err != nil || resolved.Line.EffectiveStatus != purchasecore.LinkLinked || resolved.CommercialIdentityDisposition != purchasecore.CommercialIdentityCreated {
		t.Fatalf("ResolvePurchaseLine = %#v, %v", resolved, err)
	}
	got, err := writer.SetResolutionOverride(ctx, purchasecore.SetResolutionOverrideRequest{
		LineID: 1, Override: purchasecore.LinkNotApplicable, Actor: "PI", Reason: "manual review",
	})
	if err != nil || got.EffectiveStatus != purchasecore.LinkNotApplicable {
		t.Fatalf("SetResolutionOverride = %#v, %v", got, err)
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
		{"ConfirmMapping non-positive resource id", func() error {
			_, err := writer.ConfirmMapping(ctx, purchasecore.ConfirmMappingRequest{SupplierProductID: 1})
			return err
		}},
		{"ResolvePurchaseLine missing actor", func() error {
			_, err := writer.ResolvePurchaseLine(ctx, purchasecore.ResolvePurchaseLineRequest{LineID: 1, ResourceID: 2, CommercialSupplierSKU: "SKU"})
			return err
		}},
		{"ResolvePurchaseLine incomplete existing snapshot", func() error {
			productID := int64(3)
			_, err := writer.ResolvePurchaseLine(ctx, purchasecore.ResolvePurchaseLineRequest{LineID: 1, ResourceID: 2, ExpectedSupplierProductID: &productID, Actor: "PI"})
			return err
		}},
		{"SetResolutionOverride derived status", func() error {
			_, err := writer.SetResolutionOverride(ctx, purchasecore.SetResolutionOverrideRequest{LineID: 1, Override: purchasecore.LinkPending, Actor: "PI", Reason: "x"})
			return err
		}},
		{"SetResolutionOverride missing reason", func() error {
			_, err := writer.SetResolutionOverride(ctx, purchasecore.SetResolutionOverrideRequest{LineID: 1, Override: purchasecore.LinkConflict, Actor: "PI"})
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

func TestWriter_ResolvePurchaseLine_SKUErrorsAreSpecific(t *testing.T) {
	writer, err := purchasecore.NewWriter(fakeWriteCapabilities{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, err = writer.ResolvePurchaseLine(ctx, purchasecore.ResolvePurchaseLineRequest{LineID: 1, ResourceID: 2, Actor: "PI"})
	if !purchasecore.IsCode(err, purchasecore.CommercialSupplierSKURequired) {
		t.Fatalf("missing sku error = %v", err)
	}
	productID := int64(3)
	revision := purchasecore.MappingRevision(0)
	_, err = writer.ResolvePurchaseLine(ctx, purchasecore.ResolvePurchaseLineRequest{
		LineID: 1, ResourceID: 2, Actor: "PI", CommercialSupplierSKU: "forbidden",
		ExpectedSupplierProductID: &productID, ExpectedMappingRevision: &revision,
	})
	if !purchasecore.IsCode(err, purchasecore.CommercialSupplierSKUForbidden) {
		t.Fatalf("forbidden sku error = %v", err)
	}
}

func TestNewWriter_RejectsNilCapabilities(t *testing.T) {
	if _, err := purchasecore.NewWriter(nil); !purchasecore.IsCode(err, purchasecore.InvalidArgument) {
		t.Fatalf("error = %v, want INVALID_ARGUMENT", err)
	}
}
