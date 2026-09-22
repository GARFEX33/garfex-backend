package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GARFEX33/garfex-costos-unitarios/suppliercore"
)

const supplierPreviewPath = "/v1/suppliers/from-cfdi/preview"

func servePreview(t *testing.T, reader SupplierReader, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	NewRouter(nil, nil, nil, reader, nil, nil, nil, nil).ServeHTTP(rec, req)
	return rec
}

func previewMultipart(t *testing.T, content string) *http.Request {
	t.Helper()
	req := cfdiMultipartRequest(t, "file", "factura.xml", content)
	req.URL.Path = supplierPreviewPath
	return req
}

func decodePreview(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body)
	}
	return body
}

func TestSupplierPreviewNewSupplierHasNullExisting(t *testing.T) {
	var looked string
	reader := supplierReaderFuncs{getByTaxID: func(_ context.Context, taxID string) (suppliercore.Supplier, error) {
		looked = taxID
		return suppliercore.Supplier{}, suppliercore.NewError(suppliercore.NotFound, "supplier not found")
	}}

	rec := servePreview(t, reader, previewMultipart(t, cfdiSampleXML))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if looked != "ABC010101AA1" {
		t.Fatalf("lookup used %q, want the normalized Emisor RFC", looked)
	}
	body := decodePreview(t, rec)
	validateSchemaJSON(t, catalogSchema(t, "SupplierCFDIPreview"), body)
	draft := body["draft"].(map[string]any)
	if draft["taxIdentifier"] != "ABC010101AA1" || draft["legalName"] != "PROVEEDOR SA DE CV" || draft["taxRegime"] != "601" {
		t.Fatalf("draft = %#v", draft)
	}
	if v, ok := body["existing"]; !ok || v != nil {
		t.Fatalf("existing = %#v, want explicit null", body["existing"])
	}
}

func TestSupplierPreviewReturnsExistingSupplierIncludingInactive(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	reader := supplierReaderFuncs{getByTaxID: func(context.Context, string) (suppliercore.Supplier, error) {
		return suppliercore.Supplier{ID: 42, LegalName: "PROVEEDOR SA DE CV", TaxIdentifier: "ABC010101AA1", Active: false, CreatedAt: created, UpdatedAt: created}, nil
	}}

	rec := servePreview(t, reader, previewMultipart(t, cfdiSampleXML))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	body := decodePreview(t, rec)
	validateSchemaJSON(t, catalogSchema(t, "SupplierCFDIPreview"), body)
	existing := body["existing"].(map[string]any)
	if existing["id"] != "42" || existing["active"] != false || existing["taxIdentifier"] != "ABC010101AA1" {
		t.Fatalf("existing = %#v", existing)
	}
}

func TestSupplierPreviewAcceptsRawXMLBody(t *testing.T) {
	reader := supplierReaderFuncs{getByTaxID: func(context.Context, string) (suppliercore.Supplier, error) {
		return suppliercore.Supplier{}, suppliercore.NewError(suppliercore.NotFound, "x")
	}}
	req := httptest.NewRequest(http.MethodPost, supplierPreviewPath, strings.NewReader(cfdiSampleXML))
	req.Header.Set("Content-Type", "application/xml")

	if rec := servePreview(t, reader, req); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
}

func TestSupplierPreviewRejections(t *testing.T) {
	notCalled := supplierReaderFuncs{getByTaxID: func(context.Context, string) (suppliercore.Supplier, error) {
		t.Fatal("lookup must not run for an unusable file")
		return suppliercore.Supplier{}, nil
	}}
	tests := []struct {
		name     string
		reader   SupplierReader
		req      *http.Request
		status   int
		wantCode string
	}{
		{"not xml", notCalled, previewMultipart(t, "hello"), http.StatusUnprocessableEntity, "INVALID_XML"},
		{"not a cfdi", notCalled, previewMultipart(t, "<invoice/>"), http.StatusUnprocessableEntity, "NOT_CFDI"},
		{"missing file field", notCalled, func() *http.Request {
			r := cfdiMultipartRequest(t, "other", "f.xml", cfdiSampleXML)
			r.URL.Path = supplierPreviewPath
			return r
		}(), http.StatusBadRequest, ""},
		{"reader unavailable", nil, previewMultipart(t, cfdiSampleXML), http.StatusInternalServerError, ""},
		{"lookup infrastructure failure", supplierReaderFuncs{getByTaxID: func(context.Context, string) (suppliercore.Supplier, error) {
			return suppliercore.Supplier{}, suppliercore.NewError(suppliercore.Internal, "pq: connection refused")
		}}, previewMultipart(t, cfdiSampleXML), http.StatusInternalServerError, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := servePreview(t, tt.reader, tt.req)
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tt.status, rec.Body)
			}
			var got errorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got.Code != tt.wantCode {
				t.Fatalf("code = %q, want %q", got.Code, tt.wantCode)
			}
			if strings.Contains(rec.Body.String(), "connection refused") {
				t.Fatal("infrastructure error leaked")
			}
		})
	}
}

func TestSupplierPreviewRejectsOtherMethods(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		rec := servePreview(t, nil, httptest.NewRequest(method, supplierPreviewPath, nil))
		if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != http.MethodPost {
			t.Fatalf("%s status = %d Allow = %q", method, rec.Code, rec.Header().Get("Allow"))
		}
	}
}

func TestSupplierPreviewOpenAPIShape(t *testing.T) {
	body := serveCFDI(t, httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)).Body.String()
	for _, want := range []string{"  /v1/suppliers/from-cfdi/preview:\n", "SupplierCFDIPreview"} {
		if !strings.Contains(body, want) {
			t.Errorf("OpenAPI document does not contain %q", want)
		}
	}
}
