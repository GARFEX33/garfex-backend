package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GARFEX33/garfex-backend/resourcecore"
)

func TestRouterRejectsOversizedRequestBody(t *testing.T) {
	called := false
	writer := catalogWriterFuncs{create: func(context.Context, resourcecore.CatalogWriteRequest) (resourcecore.CatalogRecord, error) {
		called = true
		return resourcecore.CatalogRecord{}, nil
	}}
	h := NewRouter(nil, nil, nil, nil, nil, writer, nil, nil)
	oversized := strings.Repeat("a", maxRequestBodyBytes+1)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/catalog/UNIDAD", strings.NewReader(oversized)))
	if r.Code != http.StatusBadRequest || called {
		t.Fatalf("status/called = %d/%t, want %d/false", r.Code, called, http.StatusBadRequest)
	}
}

func TestPublicEndpoints(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, nil)

	tests := []struct {
		name        string
		method      string
		path        string
		status      int
		contentType string
		body        string
		contains    string
	}{
		{
			name:        "liveness health check",
			method:      http.MethodGet,
			path:        "/healthz",
			status:      http.StatusOK,
			contentType: "application/json",
			body:        "{\"status\":\"ok\"}\n",
		},
		{
			name:        "embedded OpenAPI document",
			method:      http.MethodGet,
			path:        "/openapi.yaml",
			status:      http.StatusOK,
			contentType: "application/yaml",
			contains:    "openapi: 3.0.3\n",
		},
		{
			name:        "Scalar documentation page",
			method:      http.MethodGet,
			path:        "/docs",
			status:      http.StatusOK,
			contentType: "text/html",
			contains:    "@scalar/api-reference@1.25.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(tt.method, tt.path, nil))

			if r.Code != tt.status {
				t.Fatalf("status = %d, want %d", r.Code, tt.status)
			}
			if got := r.Header().Get("Content-Type"); !strings.HasPrefix(got, tt.contentType) {
				t.Errorf("Content-Type = %q, want prefix %q", got, tt.contentType)
			}
			if got := r.Header().Get("X-Content-Type-Options"); got != "nosniff" {
				t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
			}
			if tt.body != "" && r.Body.String() != tt.body {
				t.Errorf("body = %q, want %q", r.Body.String(), tt.body)
			}
			if tt.contains != "" && !strings.Contains(r.Body.String(), tt.contains) {
				t.Errorf("body does not contain %q", tt.contains)
			}
		})
	}
}

func TestRouterRejectsUnexpectedRequestsWithoutReflection(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, nil)

	tests := []struct {
		name   string
		method string
		path   string
		status int
		body   string
	}{
		{"unknown path", http.MethodGet, "/not-a-route?token=secret", http.StatusNotFound, "404 not found\n"},
		{"business-looking path", http.MethodGet, "/v1/orders", http.StatusNotFound, "404 not found\n"},
		{"wrong method", http.MethodPost, "/healthz", http.StatusMethodNotAllowed, "405 method not allowed\n"},
		{"HEAD is not an advertised method", http.MethodHead, "/docs", http.StatusMethodNotAllowed, "405 method not allowed\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(tt.method, tt.path, nil))

			if r.Code != tt.status || r.Body.String() != tt.body {
				t.Fatalf("got status %d and body %q", r.Code, r.Body.String())
			}
			if tt.status == http.StatusMethodNotAllowed && r.Header().Get("Allow") != http.MethodGet {
				t.Errorf("Allow = %q, want GET", r.Header().Get("Allow"))
			}
			if got := r.Header().Get("Access-Control-Allow-Origin"); got != "" {
				t.Errorf("unexpected CORS header: %q", got)
			}
			if got := r.Header().Get("X-Content-Type-Options"); got != "nosniff" {
				t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
			}
		})
	}
}

func TestOpenAPIDescribesOnlyImplementedBusinessPath(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil))

	body := r.Body.String()
	for _, path := range []string{
		"  /healthz:\n", "  /v1/catalog/descriptors:\n", "  /v1/catalog/classes:\n", "  /v1/catalog/{kind}:\n", "  /v1/catalog/{kind}/{id}:\n",
		"  /v1/catalog/{kind}/{id}/deactivate:\n", "  /v1/catalog/{kind}/{id}/reactivate:\n",
		"  /v1/suppliers:\n", "  /v1/suppliers/{id}:\n", "  /v1/suppliers/{id}/branches:\n", "  /v1/suppliers/{id}/branches/{branchId}:\n",
		"  /v1/suppliers/{id}/contacts:\n", "  /v1/suppliers/{id}/contacts/{contactId}:\n",
		"  /v1/resources:\n", "  /v1/resources/{classCode}/{identityV1}:\n", "  /v1/resources/{classCode}/{identityV1}/describe:\n",
		"  /v1/resources/{id}:\n", "  /v1/resources/{id}/deactivate:\n", "  /v1/resources/{id}/reactivate:\n",
		"  /v1/cfdi/parse:\n", "  /v1/suppliers/from-cfdi/preview:\n", "  /v1/types/{typeCode}/attributes/effective:\n", "  /v1/types/{typeCode}/attributes/evaluate:\n",
		"  /v1/purchase-lines:\n", "  /v1/supplier-products/{id}/mapping/confirm:\n", "  /v1/supplier-products/{id}/mapping/correct:\n",
		"  /v1/supplier-products/{id}/mapping/retire:\n", "  /v1/supplier-products/{id}/mapping/report-conflict:\n", "  /v1/supplier-products/{id}/mapping/resolve-conflict:\n",
		"  /v1/purchase-lines/{lineId}/resolve:\n", "  /v1/purchase-lines/{lineId}/resolution-override:\n",
		"  /openapi.yaml:\n", "  /docs:\n",
	} {
		if !strings.Contains(body, path) {
			t.Errorf("OpenAPI document does not contain %q", path)
		}
	}
	for _, path := range []string{
		"/v1/catalog/records",
		"  /v1/supplier-products/{id}/link:\n",
		"  /v1/supplier-products/{id}/unlink:\n",
		"  /v1/purchase-lines/{id}/link-status:\n",
		"LinkSupplierProductRequest",
		"UnlinkSupplierProductRequest",
		"SetLinkStatusRequest",
	} {
		if strings.Contains(body, path) {
			t.Errorf("OpenAPI document advertises legacy or unimplemented path/schema %q", path)
		}
	}
}

func TestDocsUsesScalarDeclarativeCDNInitialization(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/docs", nil))

	body := r.Body.String()
	for _, want := range []string{
		`<script id="api-reference" data-url="/openapi.yaml"></script>`,
		`<script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference@1.25.0"></script>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("documentation page does not contain %q", want)
		}
	}
	declaration := `<script id="api-reference" data-url="/openapi.yaml"></script>`
	loader := `<script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference@1.25.0"></script>`
	if strings.Count(body, loader) != 1 {
		t.Errorf("Scalar loader count = %d, want 1", strings.Count(body, loader))
	}
	if strings.Index(body, loader) < strings.Index(body, declaration) {
		t.Error("Scalar loader appears before its declarative configuration")
	}
	if strings.Contains(body, "createApiReference") {
		t.Error("documentation page uses an unsupported imperative Scalar initializer")
	}
}

func TestPurchaseMutationPathHelpers(t *testing.T) {
	for _, test := range []struct {
		path   string
		action string
	}{
		{"/v1/supplier-products/9/mapping/confirm", "confirm"},
		{"/v1/supplier-products/9/mapping/correct", "correct"},
		{"/v1/supplier-products/9/mapping/retire", "retire"},
		{"/v1/supplier-products/9/mapping/report-conflict", "report-conflict"},
		{"/v1/supplier-products/9/mapping/resolve-conflict", "resolve-conflict"},
	} {
		id, action, ok := supplierProductMappingPath(test.path)
		if !ok || id != "9" || action != test.action {
			t.Errorf("supplierProductMappingPath(%q) = %q, %q, %t", test.path, id, action, ok)
		}
	}
	if id, ok := purchaseLineResolvePath("/v1/purchase-lines/11/resolve"); !ok || id != "11" {
		t.Errorf("purchaseLineResolvePath = %q, %t", id, ok)
	}
	if id, ok := purchaseLineResolutionOverridePath("/v1/purchase-lines/11/resolution-override"); !ok || id != "11" {
		t.Errorf("purchaseLineResolutionOverridePath = %q, %t", id, ok)
	}
	for _, path := range []string{
		"/v1/supplier-products/9/link",
		"/v1/supplier-products/9/unlink",
		"/v1/purchase-lines/11/link-status",
		"/v1/supplier-products/9/mapping/unknown",
	} {
		if _, _, ok := supplierProductMappingPath(path); ok {
			t.Errorf("supplierProductMappingPath(%q) unexpectedly matched", path)
		}
	}
}
