package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GARFEX33/garfex-backend/suppliercore"
)

func TestUpdateSupplierMapsRequestAndResponse(t *testing.T) {
	var captured suppliercore.SupplierUpdateRequest
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	writer := supplierWriterFuncs{update: func(_ context.Context, req suppliercore.SupplierUpdateRequest) (suppliercore.Supplier, error) {
		captured = req
		return suppliercore.Supplier{
			ID: req.ID, TradeName: req.TradeName, LegalName: req.LegalName,
			TaxIdentifier: req.TaxIdentifier, Website: req.Website, Notes: req.Notes,
			Active: true, CreatedAt: created, UpdatedAt: created,
		}, nil
	}}
	h := NewRouter(nil, writer, nil, nil, nil, nil, nil, nil)
	body := `{"actor":"tester","tradeName":"Acme2","legalName":"Acme SA","taxIdentifier":"TAX2","website":"https://acme.test","notes":"n2"}`
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPut, "/v1/suppliers/42", strings.NewReader(body)))

	wantRequest := suppliercore.SupplierUpdateRequest{
		Actor: "tester", ID: 42, TradeName: "Acme2", LegalName: "Acme SA",
		TaxIdentifier: "TAX2", Website: "https://acme.test", Notes: "n2",
	}
	if captured != wantRequest {
		t.Fatalf("request = %#v, want %#v", captured, wantRequest)
	}
	if r.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", r.Code, r.Body.String())
	}
	var response supplierResponse
	if err := json.NewDecoder(r.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.ID != "42" || response.TradeName != "Acme2" {
		t.Fatalf("response = %#v", response)
	}
	validateSchemaJSON(t, catalogSchema(t, "Supplier"), response)
}

func TestUpdateSupplierRejectsInvalidIDWithoutCallingCore(t *testing.T) {
	called := false
	writer := supplierWriterFuncs{update: func(context.Context, suppliercore.SupplierUpdateRequest) (suppliercore.Supplier, error) {
		called = true
		return suppliercore.Supplier{}, nil
	}}
	h := NewRouter(nil, writer, nil, nil, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPut, "/v1/suppliers/0", strings.NewReader(`{"actor":"tester"}`)))
	var got errorResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if r.Code != http.StatusBadRequest || got.Error != "invalid request" || called {
		t.Fatalf("status/error/called = %d/%q/%t", r.Code, got.Error, called)
	}
}

func TestUpdateSupplierRejectsInvalidBodyWithoutCallingCore(t *testing.T) {
	called := false
	writer := supplierWriterFuncs{update: func(context.Context, suppliercore.SupplierUpdateRequest) (suppliercore.Supplier, error) {
		called = true
		return suppliercore.Supplier{}, nil
	}}
	h := NewRouter(nil, writer, nil, nil, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPut, "/v1/suppliers/1", strings.NewReader("{not json")))
	var got errorResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if r.Code != http.StatusBadRequest || got.Error != "invalid request" || called {
		t.Fatalf("status/error/called = %d/%q/%t", r.Code, got.Error, called)
	}
}

func TestUpdateSupplierSanitizesNilWriter(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPut, "/v1/suppliers/1", strings.NewReader(`{"actor":"tester"}`)))
	var got errorResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if r.Code != http.StatusInternalServerError || got.Error != "internal server error" {
		t.Fatalf("status/error = %d/%q", r.Code, got.Error)
	}
}

func TestSupplierUpdateRequestOpenAPIShape(t *testing.T) {
	schema := catalogSchema(t, "SupplierUpdateRequest")
	if err := schema.VisitJSON(map[string]any{"actor": "tester", "extra": true}); err == nil {
		t.Fatal("extra property matched SupplierUpdateRequest")
	}
	if err := schema.VisitJSON(map[string]any{"tradeName": "Acme"}); err == nil {
		t.Fatal("SupplierUpdateRequest matched without actor")
	}
}

func TestUpdateSupplierMapsAndSanitizesCoreErrors(t *testing.T) {
	for _, tc := range []struct {
		code    suppliercore.ErrorCode
		status  int
		message string
	}{
		{suppliercore.NotFound, 404, "not found"},
		{suppliercore.Conflict, 409, "conflict"},
		{suppliercore.Validation, 422, "validation failed"},
	} {
		t.Run(string(tc.code), func(t *testing.T) {
			writer := supplierWriterFuncs{update: func(context.Context, suppliercore.SupplierUpdateRequest) (suppliercore.Supplier, error) {
				return suppliercore.Supplier{}, suppliercore.NewError(tc.code, "postgres://user:secret@host/db")
			}}
			h := NewRouter(nil, writer, nil, nil, nil, nil, nil, nil)
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodPut, "/v1/suppliers/1", strings.NewReader(`{"actor":"tester"}`)))
			var got errorResponse
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			if r.Code != tc.status || got.Error != tc.message {
				t.Fatalf("status/error = %d/%q, want %d/%q", r.Code, got.Error, tc.status, tc.message)
			}
		})
	}
}
