package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GARFEX33/garfex-costos-unitarios/purchasecore"
)

func TestSupplierProductLink(t *testing.T) {
	var gotReq purchasecore.LinkSupplierProductRequest
	writer := purchaseWriterFuncs{linkProduct: func(_ context.Context, req purchasecore.LinkSupplierProductRequest) (purchasecore.SupplierProduct, error) {
		gotReq = req
		linked := sampleSupplierProduct
		linked.ResourceID = &req.ResourceID
		return linked, nil
	}}
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, writer)
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"actor":"tester","resourceId":"42"}`)
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/supplier-products/9/link", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if gotReq.Actor != "tester" || gotReq.SupplierProductID != 9 || gotReq.ResourceID != 42 {
		t.Fatalf("request = %#v", gotReq)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["resourceId"] != "42" {
		t.Fatalf("body = %#v", resp)
	}
}

func TestSupplierProductLinkRejectsInvalidResourceID(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, purchaseWriterFuncs{})
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"actor":"tester","resourceId":"not-a-number"}`)
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/supplier-products/9/link", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body)
	}
}

func TestSupplierProductUnlink(t *testing.T) {
	var gotReq purchasecore.UnlinkSupplierProductRequest
	writer := purchaseWriterFuncs{unlinkProduct: func(_ context.Context, req purchasecore.UnlinkSupplierProductRequest) (purchasecore.SupplierProduct, error) {
		gotReq = req
		return sampleSupplierProduct, nil
	}}
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, writer)
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"actor":"tester"}`)
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/supplier-products/9/unlink", body))
	if rec.Code != http.StatusOK || gotReq.SupplierProductID != 9 || gotReq.Actor != "tester" {
		t.Fatalf("status = %d, request = %#v, body = %s", rec.Code, gotReq, rec.Body)
	}
}

func TestSetPurchaseLineLinkStatus(t *testing.T) {
	var gotReq purchasecore.SetPurchaseLineLinkStatusRequest
	writer := purchaseWriterFuncs{setLineLinkStatus: func(_ context.Context, req purchasecore.SetPurchaseLineLinkStatusRequest) (purchasecore.PurchaseLine, error) {
		gotReq = req
		return purchasecore.PurchaseLine{ID: req.PurchaseLineID, LinkStatus: req.Status}, nil
	}}
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, writer)
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"actor":"tester","status":"NO_APLICA"}`)
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/purchase-lines/11/link-status", body))
	if rec.Code != http.StatusOK || gotReq.PurchaseLineID != 11 || gotReq.Status != purchasecore.LinkNotApplicable {
		t.Fatalf("status = %d, request = %#v, body = %s", rec.Code, gotReq, rec.Body)
	}
}

func TestSetPurchaseLineLinkStatusRejectsInvalidStatus(t *testing.T) {
	writer := purchaseWriterFuncs{setLineLinkStatus: func(context.Context, purchasecore.SetPurchaseLineLinkStatusRequest) (purchasecore.PurchaseLine, error) {
		return purchasecore.PurchaseLine{}, purchasecore.NewError(purchasecore.InvalidArgument, "link status is not a recognized value")
	}}
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, writer)
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"actor":"tester","status":"NOT_A_STATUS"}`)
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/purchase-lines/11/link-status", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body)
	}
}

func TestPurchaseLinkRoutesRejectOtherMethods(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, purchaseWriterFuncs{})
	for _, path := range []string{"/v1/supplier-products/9/link", "/v1/supplier-products/9/unlink", "/v1/purchase-lines/11/link-status"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: status = %d, want %d", path, rec.Code, http.StatusMethodNotAllowed)
		}
	}
}

func TestPurchaseLinkRoutesRejectMissingWriter(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, nil)
	for _, path := range []string{"/v1/supplier-products/9/link", "/v1/supplier-products/9/unlink", "/v1/purchase-lines/11/link-status"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"actor":"tester","resourceId":"1","status":"PENDIENTE"}`)))
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("%s: status = %d, want %d, body = %s", path, rec.Code, http.StatusInternalServerError, rec.Body)
		}
	}
}
