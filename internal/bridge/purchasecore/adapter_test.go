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
	importCFDI                  func(ctx context.Context, xml []byte, opts app.ImportOptions) (domain.ImportResult, error)
	getPurchase                 func(ctx context.Context, id int64) (domain.Purchase, error)
	getPurchaseByUUID           func(ctx context.Context, uuid string) (domain.Purchase, error)
	listPurchaseLines           func(ctx context.Context, purchaseID int64) ([]domain.PurchaseLine, error)
	listPurchasesBySupplier     func(ctx context.Context, supplierID int64, criteria domain.ListCriteria) ([]domain.Purchase, error)
	getSupplierProduct          func(ctx context.Context, id int64) (domain.SupplierProduct, error)
	findSupplierProduct         func(ctx context.Context, supplierID int64, sku string) (domain.SupplierProduct, error)
	listSupplierProducts        func(ctx context.Context, supplierID int64, criteria domain.ListCriteria) ([]domain.SupplierProduct, error)
	listPurchaseLinesByResource func(ctx context.Context, resourceID int64, criteria domain.ListCriteria) ([]domain.PurchaseLineHistory, error)
	listMappingAudit            func(ctx context.Context, supplierProductID int64, criteria domain.ListCriteria) ([]domain.MappingAuditEntry, error)
	confirmMapping              func(context.Context, domain.ConfirmMappingCommand) (domain.SupplierProduct, error)
	correctMapping              func(context.Context, domain.CorrectMappingCommand) (domain.SupplierProduct, error)
	exceptionalUnlink           func(context.Context, domain.ExceptionalUnlinkCommand) (domain.SupplierProduct, error)
	reportIdentityConflict      func(context.Context, domain.ReportIdentityConflictCommand) (domain.SupplierProduct, error)
	resolveIdentityConflict     func(context.Context, domain.ResolveIdentityConflictCommand) (domain.SupplierProduct, error)
	markNotApplicable           func(context.Context, int64) (domain.PurchaseLine, error)
	markConflict                func(context.Context, int64) (domain.PurchaseLine, error)
	clearOverride               func(context.Context, int64) (domain.PurchaseLine, error)
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
func (s *stubService) ListMappingAudit(ctx context.Context, id int64, criteria domain.ListCriteria) ([]domain.MappingAuditEntry, error) {
	return s.listMappingAudit(ctx, id, criteria)
}
func (s *stubService) ConfirmMapping(ctx context.Context, command domain.ConfirmMappingCommand) (domain.SupplierProduct, error) {
	return s.confirmMapping(ctx, command)
}
func (s *stubService) CorrectMapping(ctx context.Context, command domain.CorrectMappingCommand) (domain.SupplierProduct, error) {
	return s.correctMapping(ctx, command)
}
func (s *stubService) ExceptionalUnlink(ctx context.Context, command domain.ExceptionalUnlinkCommand) (domain.SupplierProduct, error) {
	return s.exceptionalUnlink(ctx, command)
}
func (s *stubService) ReportIdentityConflict(ctx context.Context, command domain.ReportIdentityConflictCommand) (domain.SupplierProduct, error) {
	return s.reportIdentityConflict(ctx, command)
}
func (s *stubService) ResolveIdentityConflict(ctx context.Context, command domain.ResolveIdentityConflictCommand) (domain.SupplierProduct, error) {
	return s.resolveIdentityConflict(ctx, command)
}
func (s *stubService) MarkNotApplicable(ctx context.Context, id int64) (domain.PurchaseLine, error) {
	return s.markNotApplicable(ctx, id)
}
func (s *stubService) MarkConflict(ctx context.Context, id int64) (domain.PurchaseLine, error) {
	return s.markConflict(ctx, id)
}
func (s *stubService) ClearOverride(ctx context.Context, id int64) (domain.PurchaseLine, error) {
	return s.clearOverride(ctx, id)
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

func TestMapSupplierProduct_UsesResourceActivityProjection(t *testing.T) {
	resourceID := int64(42)
	inactive := false
	got := mapSupplierProduct(domain.SupplierProduct{CurrentMapping: domain.SupplierProductMapping{ResourceID: &resourceID}, ResourceActive: &inactive})
	if got.MappingState != public.MappingStateSuspended || got.MappingCause != public.MappingCauseResourceInactive {
		t.Fatalf("inactive projection = %+v, want SUSPENDED/RESOURCE_INACTIVE", got)
	}
	active := true
	got = mapSupplierProduct(domain.SupplierProduct{CurrentMapping: domain.SupplierProductMapping{ResourceID: &resourceID}, ResourceActive: &active})
	if got.MappingState != public.MappingStateConfirmed || got.MappingCause != public.MappingCauseNone {
		t.Fatalf("active projection = %+v, want CONFIRMED/NONE", got)
	}
}

func TestAdapter_ConfirmMapping_MapsCurrentMapping(t *testing.T) {
	resourceID := int64(42)
	stub := &stubService{confirmMapping: func(ctx context.Context, command domain.ConfirmMappingCommand) (domain.SupplierProduct, error) {
		return domain.SupplierProduct{ID: command.SupplierProductID, CurrentMapping: domain.SupplierProductMapping{ResourceID: &resourceID}, MappingRevision: 1}, nil
	}}
	adapter := NewAdapter(stub)
	got, err := adapter.ConfirmMapping(context.Background(), public.ConfirmMappingRequest{SupplierProductID: 1, ResourceID: resourceID, Decision: public.MappingDecisionMetadata{Actor: "PI", Origin: public.MappingOriginManual, At: time.Now()}})
	if err != nil {
		t.Fatalf("ConfirmMapping error = %v", err)
	}
	if got.CurrentMapping.ResourceID == nil || *got.CurrentMapping.ResourceID != resourceID || got.MappingRevision != 1 {
		t.Fatalf("mapping = %+v", got)
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
