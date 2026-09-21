package purchasecore

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/GARFEX33/garfex-costos-unitarios/internal/modules/purchases/app"
	"github.com/GARFEX33/garfex-costos-unitarios/internal/modules/purchases/domain"
	supplierdomain "github.com/GARFEX33/garfex-costos-unitarios/internal/modules/suppliers/domain"
	public "github.com/GARFEX33/garfex-costos-unitarios/purchasecore"
	"github.com/shopspring/decimal"
)

type stubService struct {
	importCFDI                    func(ctx context.Context, xml []byte, opts app.ImportOptions) (domain.ImportResult, error)
	getPurchase                   func(ctx context.Context, id int64) (domain.Purchase, error)
	getPurchaseByUUID             func(ctx context.Context, uuid string) (domain.Purchase, error)
	listPurchaseLines             func(ctx context.Context, purchaseID int64) ([]domain.PurchaseLine, error)
	listPurchasesBySupplier       func(ctx context.Context, supplierID int64, criteria domain.ListCriteria) ([]domain.Purchase, error)
	getSupplierProduct            func(ctx context.Context, id int64) (domain.SupplierProduct, error)
	findSupplierProduct           func(ctx context.Context, supplierID int64, sku string) (domain.SupplierProduct, error)
	listSupplierProducts          func(ctx context.Context, supplierID int64, criteria domain.ListCriteria) ([]domain.SupplierProduct, error)
	listPurchaseLinesByResource   func(ctx context.Context, resourceID int64, criteria domain.ListCriteria) ([]domain.PurchaseLineHistory, error)
	linkSupplierProductToResource func(ctx context.Context, supplierProductID, resourceID int64) (domain.SupplierProduct, error)
	unlinkSupplierProduct         func(ctx context.Context, supplierProductID int64) (domain.SupplierProduct, error)
	setPurchaseLineLinkStatus     func(ctx context.Context, lineID int64, status domain.LinkStatus) (domain.PurchaseLine, error)
}

func (s *stubService) ImportCFDI(ctx context.Context, xml []byte, opts app.ImportOptions) (domain.ImportResult, error) {
	return s.importCFDI(ctx, xml, opts)
}
func (s *stubService) GetPurchase(ctx context.Context, id int64) (domain.Purchase, error) {
	return s.getPurchase(ctx, id)
}
func (s *stubService) GetPurchaseByUUID(ctx context.Context, uuid string) (domain.Purchase, error) {
	return s.getPurchaseByUUID(ctx, uuid)
}
func (s *stubService) ListPurchaseLines(ctx context.Context, purchaseID int64) ([]domain.PurchaseLine, error) {
	return s.listPurchaseLines(ctx, purchaseID)
}
func (s *stubService) ListPurchasesBySupplier(ctx context.Context, supplierID int64, criteria domain.ListCriteria) ([]domain.Purchase, error) {
	return s.listPurchasesBySupplier(ctx, supplierID, criteria)
}
func (s *stubService) GetSupplierProduct(ctx context.Context, id int64) (domain.SupplierProduct, error) {
	return s.getSupplierProduct(ctx, id)
}
func (s *stubService) FindSupplierProduct(ctx context.Context, supplierID int64, sku string) (domain.SupplierProduct, error) {
	return s.findSupplierProduct(ctx, supplierID, sku)
}
func (s *stubService) ListSupplierProducts(ctx context.Context, supplierID int64, criteria domain.ListCriteria) ([]domain.SupplierProduct, error) {
	return s.listSupplierProducts(ctx, supplierID, criteria)
}
func (s *stubService) ListPurchaseLinesByResource(ctx context.Context, resourceID int64, criteria domain.ListCriteria) ([]domain.PurchaseLineHistory, error) {
	return s.listPurchaseLinesByResource(ctx, resourceID, criteria)
}
func (s *stubService) LinkSupplierProductToResource(ctx context.Context, supplierProductID, resourceID int64) (domain.SupplierProduct, error) {
	return s.linkSupplierProductToResource(ctx, supplierProductID, resourceID)
}
func (s *stubService) UnlinkSupplierProduct(ctx context.Context, supplierProductID int64) (domain.SupplierProduct, error) {
	return s.unlinkSupplierProduct(ctx, supplierProductID)
}
func (s *stubService) SetPurchaseLineLinkStatus(ctx context.Context, lineID int64, status domain.LinkStatus) (domain.PurchaseLine, error) {
	return s.setPurchaseLineLinkStatus(ctx, lineID, status)
}

func samplePurchase() domain.Purchase {
	rate := decimal.NewFromInt(1)
	return domain.Purchase{
		ID:             1,
		Supplier:       domain.PurchaseParty{SupplierID: 2},
		CFDIUUID:       "ABCDEF12-3456-7890-ABCD-EF1234567890",
		Series:         "AB",
		Folio:          "1234",
		IssuedAt:       time.Date(2026, 3, 5, 9, 30, 15, 0, time.UTC),
		Currency:       "MXN",
		ExchangeRate:   &rate,
		Subtotal:       decimal.NewFromInt(100),
		Discount:       decimal.Zero,
		TaxTransferred: decimal.NewFromInt(16),
		TaxWithheld:    decimal.Zero,
		Total:          decimal.NewFromInt(116),
		IssuerTaxID:    "ABC010101AA1",
		IssuerName:     "PROVEEDOR",
		XML:            domain.XMLDocument{Content: []byte("<xml/>"), Hash: "hash", Filename: "f.xml"},
	}
}

func TestAdapter_GetPurchase_MapsAllFields(t *testing.T) {
	want := samplePurchase()
	stub := &stubService{getPurchase: func(ctx context.Context, id int64) (domain.Purchase, error) { return want, nil }}
	adapter := NewAdapter(stub)

	got, err := adapter.GetPurchase(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetPurchase error = %v", err)
	}
	if got.CFDIUUID != want.CFDIUUID || got.SupplierID != want.Supplier.SupplierID {
		t.Fatalf("mapPurchase mismatch: got %+v, want fields from %+v", got, want)
	}
	if got.Total != "116" || got.Subtotal != "100" || got.TaxTransferred != "16" {
		t.Fatalf("mapPurchase decimal mismatch: %+v", got)
	}
	if got.ExchangeRate == nil || *got.ExchangeRate != "1" {
		t.Fatalf("ExchangeRate = %v, want \"1\"", got.ExchangeRate)
	}
	if string(got.XML.Content) != "<xml/>" {
		t.Fatalf("XML.Content = %q, want <xml/>", got.XML.Content)
	}
}

func TestAdapter_GetPurchase_NotFound(t *testing.T) {
	stub := &stubService{getPurchase: func(ctx context.Context, id int64) (domain.Purchase, error) {
		return domain.Purchase{}, fmt.Errorf("get purchase: %w", domain.ErrPurchaseNotFound)
	}}
	adapter := NewAdapter(stub)

	_, err := adapter.GetPurchase(context.Background(), 1)
	if !public.IsCode(err, public.NotFound) {
		t.Fatalf("error = %v, want NOT_FOUND", err)
	}
}

func TestAdapter_ImportPurchase_ReturnsAlreadyExisted(t *testing.T) {
	want := samplePurchase()
	stub := &stubService{importCFDI: func(ctx context.Context, xml []byte, opts app.ImportOptions) (domain.ImportResult, error) {
		return domain.ImportResult{Purchase: want, AlreadyExisted: true}, nil
	}}
	adapter := NewAdapter(stub)

	result, err := adapter.ImportPurchase(context.Background(), public.ImportRequest{Actor: "PI", XML: []byte("<xml/>")})
	if err != nil {
		t.Fatalf("ImportPurchase error = %v", err)
	}
	if !result.AlreadyExisted {
		t.Fatal("expected AlreadyExisted to propagate")
	}
	if result.Purchase.CFDIUUID != want.CFDIUUID {
		t.Fatalf("Purchase.CFDIUUID = %q, want %q", result.Purchase.CFDIUUID, want.CFDIUUID)
	}
}

func TestAdapter_ImportPurchase_MapsConflict(t *testing.T) {
	stub := &stubService{importCFDI: func(ctx context.Context, xml []byte, opts app.ImportOptions) (domain.ImportResult, error) {
		return domain.ImportResult{}, fmt.Errorf("import purchase: %w", domain.ErrPurchaseConflict)
	}}
	adapter := NewAdapter(stub)

	_, err := adapter.ImportPurchase(context.Background(), public.ImportRequest{Actor: "PI", XML: []byte("<xml/>")})
	if !public.IsCode(err, public.Conflict) {
		t.Fatalf("error = %v, want CONFLICT", err)
	}
}

func TestAdapter_ImportPurchase_MapsSupplierMasterErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want public.ErrorCode
	}{
		{"supplier branch not found", supplierdomain.ErrBranchNotFound, public.NotFound},
		{"supplier validation failed", supplierdomain.ErrValidation, public.Validation},
		{"supplier conflict", supplierdomain.ErrConflict, public.Conflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &stubService{importCFDI: func(ctx context.Context, xml []byte, opts app.ImportOptions) (domain.ImportResult, error) {
				return domain.ImportResult{}, fmt.Errorf("resolve purchase supplier: %w", tt.err)
			}}
			adapter := NewAdapter(stub)

			_, err := adapter.ImportPurchase(context.Background(), public.ImportRequest{Actor: "PI", XML: []byte("<xml/>")})
			if !public.IsCode(err, tt.want) {
				t.Fatalf("error = %v, want %s", err, tt.want)
			}
		})
	}
}

func TestAdapter_ImportPurchase_MapsUnclassifiedErrorToInternal(t *testing.T) {
	stub := &stubService{importCFDI: func(ctx context.Context, xml []byte, opts app.ImportOptions) (domain.ImportResult, error) {
		return domain.ImportResult{}, errors.New("boom")
	}}
	adapter := NewAdapter(stub)

	_, err := adapter.ImportPurchase(context.Background(), public.ImportRequest{Actor: "PI", XML: []byte("<xml/>")})
	if !public.IsCode(err, public.Internal) {
		t.Fatalf("error = %v, want INTERNAL", err)
	}
}

func TestAdapter_ListPurchasesBySupplier_DerivesHasNext(t *testing.T) {
	stub := &stubService{listPurchasesBySupplier: func(ctx context.Context, supplierID int64, criteria domain.ListCriteria) ([]domain.Purchase, error) {
		if criteria.Limit != 2 {
			t.Fatalf("Limit = %d, want 2 (requested 1 + over-fetch)", criteria.Limit)
		}
		return []domain.Purchase{samplePurchase(), samplePurchase()}, nil
	}}
	adapter := NewAdapter(stub)

	page, err := adapter.ListPurchasesBySupplier(context.Background(), 2, public.ListCriteria{Limit: 1})
	if err != nil {
		t.Fatalf("ListPurchasesBySupplier error = %v", err)
	}
	if len(page.Purchases) != 1 {
		t.Fatalf("Purchases = %d, want 1 (trimmed to the requested limit)", len(page.Purchases))
	}
	if !page.HasNext {
		t.Fatal("expected HasNext true when the over-fetch returns one extra row")
	}
}

func TestAdapter_LinkSupplierProductToResource_MapsFields(t *testing.T) {
	resourceID := int64(42)
	stub := &stubService{linkSupplierProductToResource: func(ctx context.Context, supplierProductID, gotResourceID int64) (domain.SupplierProduct, error) {
		return domain.SupplierProduct{
			ID:             supplierProductID,
			CurrentMapping: domain.SupplierProductMapping{ResourceID: &gotResourceID},
		}, nil
	}}
	adapter := NewAdapter(stub)

	got, err := adapter.LinkSupplierProductToResource(context.Background(), public.LinkSupplierProductRequest{Actor: "PI", SupplierProductID: 1, ResourceID: resourceID})
	if err != nil {
		t.Fatalf("LinkSupplierProductToResource error = %v", err)
	}
	if got.ResourceID == nil || *got.ResourceID != resourceID {
		t.Fatalf("ResourceID = %v, want %d", got.ResourceID, resourceID)
	}
}

func TestMapError_Categories(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want public.ErrorCode
	}{
		{"purchase not found", domain.ErrPurchaseNotFound, public.NotFound},
		{"purchase conflict", domain.ErrPurchaseConflict, public.Conflict},
		{"purchase validation", domain.ErrValidation, public.Validation},
		{"supplier not found", supplierdomain.ErrSupplierNotFound, public.NotFound},
		{"supplier validation", supplierdomain.ErrValidation, public.Validation},
		{"supplier conflict", supplierdomain.ErrConflict, public.Conflict},
		{"unclassified", errors.New("boom"), public.Internal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := public.Code(mapError(tt.err)); got != tt.want {
				t.Fatalf("mapError(%v) code = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}
