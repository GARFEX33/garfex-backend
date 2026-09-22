package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/GARFEX33/garfex-costos-unitarios/purchasecore"
)

type purchaseReaderFuncs struct {
	getPurchase                func(context.Context, int64) (purchasecore.Purchase, error)
	getPurchaseByUUID          func(context.Context, string) (purchasecore.Purchase, error)
	listPurchaseLines          func(context.Context, int64) ([]purchasecore.PurchaseLine, error)
	listPurchaseLinesWorkbench func(context.Context, purchasecore.PurchaseLineQuery) (purchasecore.PurchaseLinePage, error)
	listPurchasesBySupplier    func(context.Context, int64, purchasecore.ListCriteria) (purchasecore.PurchasePage, error)
	getSupplierProduct         func(context.Context, int64) (purchasecore.SupplierProduct, error)
	findSupplierProduct        func(context.Context, int64, string) (purchasecore.SupplierProduct, error)
	listSupplierProducts       func(context.Context, int64, purchasecore.ListCriteria) (purchasecore.SupplierProductPage, error)
	listByResource             func(context.Context, int64, purchasecore.ListCriteria) (purchasecore.PurchaseLineHistoryPage, error)
}

func (f purchaseReaderFuncs) GetPurchase(ctx context.Context, id int64) (purchasecore.Purchase, error) {
	return f.getPurchase(ctx, id)
}

func (f purchaseReaderFuncs) GetPurchaseByUUID(ctx context.Context, uuid string) (purchasecore.Purchase, error) {
	return f.getPurchaseByUUID(ctx, uuid)
}

func (f purchaseReaderFuncs) ListPurchaseLines(ctx context.Context, purchaseID int64) ([]purchasecore.PurchaseLine, error) {
	return f.listPurchaseLines(ctx, purchaseID)
}

func (f purchaseReaderFuncs) ListPurchaseLinesWorkbench(ctx context.Context, q purchasecore.PurchaseLineQuery) (purchasecore.PurchaseLinePage, error) {
	return f.listPurchaseLinesWorkbench(ctx, q)
}

func (f purchaseReaderFuncs) ListPurchasesBySupplier(ctx context.Context, supplierID int64, q purchasecore.ListCriteria) (purchasecore.PurchasePage, error) {
	return f.listPurchasesBySupplier(ctx, supplierID, q)
}

func (f purchaseReaderFuncs) GetSupplierProduct(ctx context.Context, id int64) (purchasecore.SupplierProduct, error) {
	return f.getSupplierProduct(ctx, id)
}

func (f purchaseReaderFuncs) FindSupplierProduct(ctx context.Context, supplierID int64, sku string) (purchasecore.SupplierProduct, error) {
	return f.findSupplierProduct(ctx, supplierID, sku)
}

func (f purchaseReaderFuncs) ListSupplierProducts(ctx context.Context, supplierID int64, q purchasecore.ListCriteria) (purchasecore.SupplierProductPage, error) {
	return f.listSupplierProducts(ctx, supplierID, q)
}

func (f purchaseReaderFuncs) ListPurchaseLinesByResource(ctx context.Context, resourceID int64, q purchasecore.ListCriteria) (purchasecore.PurchaseLineHistoryPage, error) {
	return f.listByResource(ctx, resourceID, q)
}

var samplePurchase = purchasecore.Purchase{
	ID:             7,
	SupplierID:     3,
	CFDIUUID:       "ABCDEF12-3456-7890-ABCD-EF1234567890",
	Series:         "AB",
	Folio:          "7",
	IssuedAt:       time.Date(2026, 3, 5, 9, 30, 15, 0, time.UTC),
	Currency:       "MXN",
	Subtotal:       "100.00",
	Discount:       "0",
	TaxTransferred: "16.00",
	TaxWithheld:    "0",
	Total:          "116.00",
	IssuerTaxID:    "ABC010101AA1",
	IssuerName:     "PROVEEDOR SA DE CV",
	XML:            purchasecore.XMLDocument{Hash: "deadbeef", Filename: "factura.xml"},
	ImportedAt:     time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC),
	CreatedAt:      time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC),
	UpdatedAt:      time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC),
}

func TestGetPurchaseReturnsMappedPurchase(t *testing.T) {
	var got int64
	reader := purchaseReaderFuncs{getPurchase: func(_ context.Context, id int64) (purchasecore.Purchase, error) {
		got = id
		return samplePurchase, nil
	}}
	h := NewRouter(nil, nil, nil, nil, nil, nil, reader, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/purchases/7", nil))
	if rec.Code != http.StatusOK || got != 7 {
		t.Fatalf("status = %d, got id = %d, body = %s", rec.Code, got, rec.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["id"] != "7" || body["cfdiUuid"] != samplePurchase.CFDIUUID {
		t.Fatalf("body = %#v", body)
	}
}

func TestGetPurchaseRejectsInvalidID(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil, purchaseReaderFuncs{}, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/purchases/0", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetPurchaseNotFound(t *testing.T) {
	reader := purchaseReaderFuncs{getPurchase: func(context.Context, int64) (purchasecore.Purchase, error) {
		return purchasecore.Purchase{}, purchasecore.NewError(purchasecore.NotFound, "not found")
	}}
	h := NewRouter(nil, nil, nil, nil, nil, nil, reader, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/purchases/9", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGetPurchaseByUUID(t *testing.T) {
	var got string
	reader := purchaseReaderFuncs{getPurchaseByUUID: func(_ context.Context, uuid string) (purchasecore.Purchase, error) {
		got = uuid
		return samplePurchase, nil
	}}
	h := NewRouter(nil, nil, nil, nil, nil, nil, reader, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/purchases/by-uuid/"+samplePurchase.CFDIUUID, nil))
	if rec.Code != http.StatusOK || got != samplePurchase.CFDIUUID {
		t.Fatalf("status = %d, got uuid = %q, body = %s", rec.Code, got, rec.Body)
	}
}

func TestListPurchaseLines(t *testing.T) {
	lines := []purchasecore.PurchaseLine{
		{ID: 1, PurchaseID: 7, LineNumber: 1, Description: "CABLE", SupplierSKU: "CAB-1", Quantity: "10", UnitPrice: "10.00", Amount: "100.00", EffectiveStatus: purchasecore.LinkPending},
	}
	reader := purchaseReaderFuncs{listPurchaseLines: func(_ context.Context, purchaseID int64) ([]purchasecore.PurchaseLine, error) {
		if purchaseID != 7 {
			t.Fatalf("purchaseID = %d, want 7", purchaseID)
		}
		return lines, nil
	}}
	h := NewRouter(nil, nil, nil, nil, nil, nil, reader, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/purchases/7/lines", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	var body []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 1 || body[0]["supplierSku"] != "CAB-1" || body[0]["effectiveStatus"] != "PENDIENTE" {
		t.Fatalf("body = %#v", body)
	}
}

func TestListPurchaseLinesWorkbenchForwardsQueryAndMapsPage(t *testing.T) {
	productID := int64(17)
	mappingRevision := purchasecore.MappingRevision(4)
	resourceID := int64(23)
	resourceIdentity := "ELEC-001"
	resourceDisplayName := "Cable eléctrico"
	issuedAt := time.Date(2026, 3, 5, 14, 30, 0, 0, time.UTC)
	wantQuery := purchasecore.PurchaseLineQuery{
		Limit:           12,
		Offset:          5,
		SupplierID:      int64Pointer(42),
		EffectiveStatus: purchasecore.LinkConflict,
		DateFrom:        timePointer(time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)),
		DateTo:          timePointer(time.Date(2026, 3, 7, 23, 59, 59, 999999999, time.UTC)),
		InvoiceText:     "Factura 7",
		SupplierSKU:     "SKU-X",
		Description:     "CABLE",
	}
	rows := []purchasecore.PurchaseLineRow{
		{
			LineID:                9,
			LineNumber:            2,
			PurchaseID:            7,
			IssuedAt:              issuedAt,
			Series:                "A",
			Folio:                 "7",
			CFDIUUID:              samplePurchase.CFDIUUID,
			SupplierID:            42,
			SupplierDisplayName:   "Proveedor",
			Description:           "CABLE",
			SupplierSKU:           "SKU-X",
			CommercialSupplierSKU: stringPointer("COMM-X"),
			SATProductCode:        "3912",
			Quantity:              "3",
			UnitCode:              "H87",
			Unit:                  "PIEZA",
			UnitPrice:             "10.00",
			Amount:                "30.00",
			Currency:              "MXN",
			SupplierProductID:     &productID,
			MappingRevision:       &mappingRevision,
			ResolutionRevision:    6,
			ResourceID:            &resourceID,
			ResourceIdentity:      &resourceIdentity,
			ResourceDisplayName:   &resourceDisplayName,
			ResolutionOverride:    purchasecore.LinkConflict,
			EffectiveStatus:       purchasecore.LinkConflict,
			EffectiveCause:        "MANUAL_OVERRIDE",
		},
		{LineID: 8, PurchaseID: 7, LineNumber: 1, IssuedAt: issuedAt, ResolutionOverride: purchasecore.LinkStatusNone, EffectiveStatus: purchasecore.LinkPending, EffectiveCause: purchasecore.MappingCauseUnresolved},
	}
	var gotQuery purchasecore.PurchaseLineQuery
	reader := purchaseReaderFuncs{listPurchaseLinesWorkbench: func(_ context.Context, q purchasecore.PurchaseLineQuery) (purchasecore.PurchaseLinePage, error) {
		gotQuery = q
		return purchasecore.PurchaseLinePage{Rows: rows, HasPrevious: true, HasNext: false}, nil
	}}
	h := NewRouter(nil, nil, nil, nil, nil, nil, reader, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/purchase-lines?supplierId=42&status=CONFLICTO&dateFrom=2026-03-05&dateTo=2026-03-07&invoice=Factura+7&supplierSku=SKU-X&description=CABLE&limit=12&offset=5", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if gotQuery.Limit != wantQuery.Limit || gotQuery.Offset != wantQuery.Offset || gotQuery.EffectiveStatus != wantQuery.EffectiveStatus || gotQuery.InvoiceText != wantQuery.InvoiceText || gotQuery.SupplierSKU != wantQuery.SupplierSKU || gotQuery.Description != wantQuery.Description {
		t.Fatalf("query = %#v, want %#v", gotQuery, wantQuery)
	}
	if gotQuery.SupplierID == nil || *gotQuery.SupplierID != *wantQuery.SupplierID || !gotQuery.DateFrom.Equal(*wantQuery.DateFrom) || !gotQuery.DateTo.Equal(*wantQuery.DateTo) {
		t.Fatalf("query dates/ids = %#v, want %#v", gotQuery, wantQuery)
	}
	var body purchaseLineWorkbenchPageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.HasPrevious || body.HasNext || len(body.Lines) != 2 {
		t.Fatalf("page = %#v", body)
	}
	if !reflect.DeepEqual(body.Lines[0], mapPurchaseLineWorkbenchRow(rows[0])) || !reflect.DeepEqual(body.Lines[1], mapPurchaseLineWorkbenchRow(rows[1])) {
		t.Fatalf("lines = %#v", body.Lines)
	}
	validateSchemaJSON(t, catalogSchema(t, "PurchaseLineWorkbenchPage"), body)
}

func TestListPurchaseLinesWorkbenchRejectsInvalidQueryWithoutCoreCall(t *testing.T) {
	for _, test := range []struct {
		name  string
		query string
	}{
		{name: "supplier id zero", query: "supplierId=0"},
		{name: "supplier id leading zero", query: "supplierId=01"},
		{name: "supplier id decimal", query: "supplierId=1.5"},
		{name: "status", query: "status=UNKNOWN"},
		{name: "date from", query: "dateFrom=2026-02-30"},
		{name: "date to", query: "dateTo=not-a-date"},
		{name: "date range", query: "dateFrom=2026-03-07&dateTo=2026-03-05"},
		{name: "limit zero", query: "limit=0"},
		{name: "limit above maximum", query: "limit=51"},
		{name: "limit malformed", query: "limit=large"},
		{name: "offset negative", query: "offset=-1"},
		{name: "offset malformed", query: "offset=large"},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			reader := purchaseReaderFuncs{listPurchaseLinesWorkbench: func(context.Context, purchasecore.PurchaseLineQuery) (purchasecore.PurchaseLinePage, error) {
				calls++
				return purchasecore.PurchaseLinePage{}, nil
			}}
			h := NewRouter(nil, nil, nil, nil, nil, nil, reader, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/purchase-lines?"+test.query, nil))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body)
			}
			if calls != 0 {
				t.Fatalf("Core calls = %d, want 0", calls)
			}
		})
	}
}

func TestListPurchaseLinesWorkbenchMethodMissingReaderAndCoreError(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil, purchaseReaderFuncs{}, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/purchase-lines", nil))
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("method mismatch = status %d, allow %q", rec.Code, rec.Header().Get("Allow"))
	}

	rec = httptest.NewRecorder()
	h = NewRouter(nil, nil, nil, nil, nil, nil, nil, nil)
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/purchase-lines", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("missing reader status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	h = NewRouter(nil, nil, nil, nil, nil, nil, purchaseReaderFuncs{listPurchaseLinesWorkbench: func(context.Context, purchasecore.PurchaseLineQuery) (purchasecore.PurchaseLinePage, error) {
		return purchasecore.PurchaseLinePage{}, purchasecore.NewError(purchasecore.Conflict, "core failure")
	}}, nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/purchase-lines", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("Core error status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func int64Pointer(value int64) *int64 { return &value }

func stringPointer(value string) *string { return &value }

func timePointer(value time.Time) *time.Time { return &value }

func TestListPurchasesBySupplierPaginates(t *testing.T) {
	var gotSupplierID int64
	var gotCriteria purchasecore.ListCriteria
	reader := purchaseReaderFuncs{listPurchasesBySupplier: func(_ context.Context, supplierID int64, q purchasecore.ListCriteria) (purchasecore.PurchasePage, error) {
		gotSupplierID, gotCriteria = supplierID, q
		return purchasecore.PurchasePage{Purchases: []purchasecore.Purchase{samplePurchase}, HasNext: true}, nil
	}}
	h := NewRouter(nil, nil, nil, nil, nil, nil, reader, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/suppliers/3/purchases?limit=10&offset=5", nil))
	if rec.Code != http.StatusOK || gotSupplierID != 3 || gotCriteria.Limit != 10 || gotCriteria.Offset != 5 {
		t.Fatalf("status = %d, supplierID = %d, criteria = %#v, body = %s", rec.Code, gotSupplierID, gotCriteria, rec.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["hasNext"] != true {
		t.Fatalf("body = %#v", body)
	}
}

func TestListPurchasesBySupplierRejectsInvalidLimit(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil, purchaseReaderFuncs{}, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/suppliers/3/purchases?limit=0", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

var sampleSupplierProduct = purchasecore.SupplierProduct{
	ID:          9,
	SupplierID:  3,
	SupplierSKU: "CAB-1",
	Description: "CABLE",
	CreatedAt:   time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC),
	UpdatedAt:   time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC),
}

func TestGetSupplierProduct(t *testing.T) {
	reader := purchaseReaderFuncs{getSupplierProduct: func(context.Context, int64) (purchasecore.SupplierProduct, error) {
		return sampleSupplierProduct, nil
	}}
	h := NewRouter(nil, nil, nil, nil, nil, nil, reader, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/supplier-products/9", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
}

func TestFindSupplierProductBySKU(t *testing.T) {
	var gotSupplierID int64
	var gotSKU string
	reader := purchaseReaderFuncs{findSupplierProduct: func(_ context.Context, supplierID int64, sku string) (purchasecore.SupplierProduct, error) {
		gotSupplierID, gotSKU = supplierID, sku
		return sampleSupplierProduct, nil
	}}
	h := NewRouter(nil, nil, nil, nil, nil, nil, reader, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/suppliers/3/products/find?sku=CAB-1", nil))
	if rec.Code != http.StatusOK || gotSupplierID != 3 || gotSKU != "CAB-1" {
		t.Fatalf("status = %d, supplierID = %d, sku = %q, body = %s", rec.Code, gotSupplierID, gotSKU, rec.Body)
	}
}

func TestListSupplierProducts(t *testing.T) {
	reader := purchaseReaderFuncs{listSupplierProducts: func(context.Context, int64, purchasecore.ListCriteria) (purchasecore.SupplierProductPage, error) {
		return purchasecore.SupplierProductPage{Products: []purchasecore.SupplierProduct{sampleSupplierProduct}}, nil
	}}
	h := NewRouter(nil, nil, nil, nil, nil, nil, reader, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/suppliers/3/products", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	products, ok := body["products"].([]any)
	if !ok || len(products) != 1 {
		t.Fatalf("body = %#v", body)
	}
}

func TestListPurchaseLinesByResource(t *testing.T) {
	history := purchasecore.PurchaseLineHistory{
		Line:            purchasecore.PurchaseLine{ID: 1, PurchaseID: 7, EffectiveStatus: purchasecore.LinkLinked},
		SupplierProduct: sampleSupplierProduct,
		PurchaseID:      7,
		PurchaseUUID:    samplePurchase.CFDIUUID,
		SupplierID:      3,
		IssuedAt:        samplePurchase.IssuedAt,
		Currency:        "MXN",
	}
	var gotResourceID int64
	reader := purchaseReaderFuncs{listByResource: func(_ context.Context, resourceID int64, _ purchasecore.ListCriteria) (purchasecore.PurchaseLineHistoryPage, error) {
		gotResourceID = resourceID
		return purchasecore.PurchaseLineHistoryPage{History: []purchasecore.PurchaseLineHistory{history}}, nil
	}}
	h := NewRouter(nil, nil, nil, nil, nil, nil, reader, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/resources/42/purchase-history", nil))
	if rec.Code != http.StatusOK || gotResourceID != 42 {
		t.Fatalf("status = %d, resourceID = %d, body = %s", rec.Code, gotResourceID, rec.Body)
	}
}

func TestPurchaseReadRoutesRejectOtherMethods(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil, purchaseReaderFuncs{}, nil)
	for _, path := range []string{
		"/v1/purchases/7", "/v1/purchases/by-uuid/x", "/v1/purchases/7/lines", "/v1/purchase-lines",
		"/v1/suppliers/3/purchases", "/v1/suppliers/3/products", "/v1/suppliers/3/products/find",
		"/v1/supplier-products/9", "/v1/resources/42/purchase-history",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, path, nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: status = %d, want %d", path, rec.Code, http.StatusMethodNotAllowed)
		}
	}
}

func TestPurchaseReadRoutesRejectMissingReader(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, nil)
	for _, path := range []string{
		"/v1/purchases/7", "/v1/purchases/by-uuid/x", "/v1/purchases/7/lines", "/v1/purchase-lines",
		"/v1/suppliers/3/purchases", "/v1/suppliers/3/products", "/v1/suppliers/3/products/find?sku=x",
		"/v1/supplier-products/9", "/v1/resources/42/purchase-history",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("%s: status = %d, want %d", path, rec.Code, http.StatusInternalServerError)
		}
	}
}
