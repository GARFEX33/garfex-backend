package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GARFEX33/garfex-costos-unitarios/purchasecore"
)

type purchaseWriterFuncs struct {
	importPurchase    func(context.Context, purchasecore.ImportRequest) (purchasecore.ImportResult, error)
	linkProduct       func(context.Context, purchasecore.LinkSupplierProductRequest) (purchasecore.SupplierProduct, error)
	unlinkProduct     func(context.Context, purchasecore.UnlinkSupplierProductRequest) (purchasecore.SupplierProduct, error)
	setLineLinkStatus func(context.Context, purchasecore.SetPurchaseLineLinkStatusRequest) (purchasecore.PurchaseLine, error)
}

func (f purchaseWriterFuncs) ImportPurchase(ctx context.Context, req purchasecore.ImportRequest) (purchasecore.ImportResult, error) {
	return f.importPurchase(ctx, req)
}

func (f purchaseWriterFuncs) LinkSupplierProductToResource(ctx context.Context, req purchasecore.LinkSupplierProductRequest) (purchasecore.SupplierProduct, error) {
	return f.linkProduct(ctx, req)
}

func (f purchaseWriterFuncs) UnlinkSupplierProduct(ctx context.Context, req purchasecore.UnlinkSupplierProductRequest) (purchasecore.SupplierProduct, error) {
	return f.unlinkProduct(ctx, req)
}

func (f purchaseWriterFuncs) SetPurchaseLineLinkStatus(ctx context.Context, req purchasecore.SetPurchaseLineLinkStatusRequest) (purchasecore.PurchaseLine, error) {
	return f.setLineLinkStatus(ctx, req)
}

func purchaseImportRequest(t *testing.T, fields map[string]string, includeFile bool) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if includeFile {
		part, err := mw.CreateFormFile("file", "factura.xml")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte(cfdiSampleXML)); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/purchases", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func TestPurchaseImportCreated(t *testing.T) {
	var gotReq purchasecore.ImportRequest
	writer := purchaseWriterFuncs{importPurchase: func(_ context.Context, req purchasecore.ImportRequest) (purchasecore.ImportResult, error) {
		gotReq = req
		return purchasecore.ImportResult{Purchase: samplePurchase, Lines: nil, AlreadyExisted: false}, nil
	}}
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, writer)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, purchaseImportRequest(t, map[string]string{"actor": "tester", "branchId": "5"}, true))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if gotReq.Actor != "tester" || gotReq.BranchID != 5 || gotReq.Filename != "factura.xml" || len(gotReq.XML) == 0 {
		t.Fatalf("import request = %#v", gotReq)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["alreadyExisted"] != false {
		t.Fatalf("body = %#v", body)
	}
}

func TestPurchaseImportAlreadyExistedReturnsOK(t *testing.T) {
	writer := purchaseWriterFuncs{importPurchase: func(context.Context, purchasecore.ImportRequest) (purchasecore.ImportResult, error) {
		return purchasecore.ImportResult{Purchase: samplePurchase, AlreadyExisted: true}, nil
	}}
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, writer)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, purchaseImportRequest(t, map[string]string{"actor": "tester"}, true))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
}

func TestPurchaseImportRejectsMissingFile(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, purchaseWriterFuncs{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, purchaseImportRequest(t, map[string]string{"actor": "tester"}, false))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body)
	}
}

func TestPurchaseImportRejectsInvalidBranchID(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, purchaseWriterFuncs{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, purchaseImportRequest(t, map[string]string{"actor": "tester", "branchId": "not-a-number"}, true))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body)
	}
}

func TestPurchaseImportPropagatesCoreValidation(t *testing.T) {
	writer := purchaseWriterFuncs{importPurchase: func(context.Context, purchasecore.ImportRequest) (purchasecore.ImportResult, error) {
		return purchasecore.ImportResult{}, purchasecore.NewError(purchasecore.InvalidArgument, "actor is required")
	}}
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, writer)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, purchaseImportRequest(t, map[string]string{"actor": ""}, true))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body)
	}
}

func TestPurchaseImportRejectsOtherMethods(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, purchaseWriterFuncs{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/purchases", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestPurchaseImportRejectsMissingWriter(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, purchaseImportRequest(t, map[string]string{"actor": "tester"}, true))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}
