package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GARFEX33/garfex-costos-unitarios/suppliercore"
)

type supplierWriterFuncs struct {
	create func(context.Context, suppliercore.SupplierWriteRequest) (suppliercore.Supplier, error)
	update func(context.Context, suppliercore.SupplierUpdateRequest) (suppliercore.Supplier, error)
}

func (f supplierWriterFuncs) CreateSupplier(ctx context.Context, req suppliercore.SupplierWriteRequest) (suppliercore.Supplier, error) {
	return f.create(ctx, req)
}

func (f supplierWriterFuncs) UpdateSupplier(ctx context.Context, req suppliercore.SupplierUpdateRequest) (suppliercore.Supplier, error) {
	return f.update(ctx, req)
}

func TestCreateSupplierMapsRequestAndResponse(t *testing.T) {
	var captured suppliercore.SupplierWriteRequest
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	updated := created.Add(time.Hour)
	h := NewRouter(nil, supplierWriterFuncs{create: func(_ context.Context, req suppliercore.SupplierWriteRequest) (suppliercore.Supplier, error) {
		captured = req
		return suppliercore.Supplier{
			ID: 9007199254740993, TradeName: req.TradeName, LegalName: req.LegalName,
			TaxIdentifier: req.TaxIdentifier, Website: req.Website, Notes: req.Notes,
			Active: true, CreatedAt: created, UpdatedAt: updated,
		}, nil
	}}, nil, nil, nil)
	body := `{"actor":"tester","tradeName":"Acme","legalName":"Acme SA","taxIdentifier":"TAX1","website":"https://acme.test","notes":"n"}`
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/suppliers", strings.NewReader(body)))

	wantRequest := suppliercore.SupplierWriteRequest{
		Actor: "tester", TradeName: "Acme", LegalName: "Acme SA",
		TaxIdentifier: "TAX1", Website: "https://acme.test", Notes: "n",
	}
	if captured != wantRequest {
		t.Fatalf("request = %#v, want %#v", captured, wantRequest)
	}
	if r.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", r.Code)
	}
	var response supplierResponse
	if err := json.NewDecoder(r.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	wantResponse := supplierResponse{
		ID: "9007199254740993", TradeName: "Acme", LegalName: "Acme SA", TaxIdentifier: "TAX1",
		Website: "https://acme.test", Notes: "n", Active: true,
		CreatedAt: "2026-01-02T03:04:05Z", UpdatedAt: "2026-01-02T04:04:05Z",
	}
	if response != wantResponse {
		t.Fatalf("response = %#v, want %#v", response, wantResponse)
	}
	validateSchemaJSON(t, catalogSchema(t, "Supplier"), response)
}

func TestSuppliersRejectsUnsupportedMethod(t *testing.T) {
	called := false
	h := NewRouter(nil, supplierWriterFuncs{create: func(context.Context, suppliercore.SupplierWriteRequest) (suppliercore.Supplier, error) {
		called = true
		return suppliercore.Supplier{}, nil
	}}, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPatch, "/v1/suppliers", nil))
	var got errorResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if r.Code != http.StatusMethodNotAllowed || r.Header().Get("Allow") != "GET, POST" || got.Error != "method not allowed" || called {
		t.Fatalf("status/allow/error/called = %d/%q/%q/%t", r.Code, r.Header().Get("Allow"), got.Error, called)
	}
}

func TestCreateSupplierRejectsInvalidBodyWithoutCallingCore(t *testing.T) {
	called := false
	h := NewRouter(nil, supplierWriterFuncs{create: func(context.Context, suppliercore.SupplierWriteRequest) (suppliercore.Supplier, error) {
		called = true
		return suppliercore.Supplier{}, nil
	}}, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/suppliers", strings.NewReader("{not json")))
	var got errorResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if r.Code != http.StatusBadRequest || got.Error != "invalid request" || called {
		t.Fatalf("status/error/called = %d/%q/%t", r.Code, got.Error, called)
	}
}

func TestCreateSupplierSanitizesNilWriter(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/suppliers", strings.NewReader(`{"actor":"tester"}`)))
	var got errorResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if r.Code != http.StatusInternalServerError || got.Error != "internal server error" {
		t.Fatalf("status/error = %d/%q", r.Code, got.Error)
	}
}

func TestCreateSupplierMapsAndSanitizesCoreErrors(t *testing.T) {
	for _, tc := range []struct {
		code    suppliercore.ErrorCode
		status  int
		message string
	}{
		{suppliercore.InvalidArgument, 400, "invalid request"},
		{suppliercore.NotFound, 404, "not found"},
		{suppliercore.Conflict, 409, "conflict"},
		{suppliercore.Validation, 422, "validation failed"},
		{suppliercore.Internal, 500, "internal server error"},
	} {
		t.Run(string(tc.code), func(t *testing.T) {
			h := NewRouter(nil, supplierWriterFuncs{create: func(context.Context, suppliercore.SupplierWriteRequest) (suppliercore.Supplier, error) {
				return suppliercore.Supplier{}, suppliercore.NewError(tc.code, "postgres://user:secret@host/db")
			}}, nil, nil, nil)
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/suppliers", strings.NewReader(`{"actor":"tester"}`)))
			var got errorResponse
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			if r.Code != tc.status || got.Error != tc.message {
				t.Fatalf("status/error = %d/%q, want %d/%q", r.Code, got.Error, tc.status, tc.message)
			}
		})
	}
	if status, _ := supplierError(errors.New("secret")); status != http.StatusInternalServerError {
		t.Fatalf("unknown error status = %d, want 500", status)
	}
}

func TestSupplierOpenAPIRejectsExtraProperties(t *testing.T) {
	if err := catalogSchema(t, "SupplierCreateRequest").VisitJSON(map[string]any{"actor": "tester", "extra": true}); err == nil {
		t.Fatal("extra property matched SupplierCreateRequest")
	}
	if err := catalogSchema(t, "Supplier").VisitJSON(map[string]any{
		"id": "1", "tradeName": "", "legalName": "", "taxIdentifier": "", "website": "", "notes": "",
		"active": true, "createdAt": "2026-01-02T03:04:05Z", "updatedAt": "2026-01-02T03:04:05Z", "extra": true,
	}); err == nil {
		t.Fatal("extra property matched Supplier")
	}
}

func TestSupplierCreateRequestRequiresActor(t *testing.T) {
	if err := catalogSchema(t, "SupplierCreateRequest").VisitJSON(map[string]any{"tradeName": "Acme"}); err == nil {
		t.Fatal("SupplierCreateRequest matched without actor")
	}
}
