package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/GARFEX33/garfex-backend/resourcecore"
)

func TestUpdateCatalogMapsRequestAndResponse(t *testing.T) {
	var captured resourcecore.CatalogUpdateRequest
	writer := catalogWriterFuncs{update: func(_ context.Context, req resourcecore.CatalogUpdateRequest) (resourcecore.CatalogRecord, error) {
		captured = req
		return resourcecore.CatalogRecord{Kind: req.Kind, ID: req.ID, Revision: req.ExpectedRevision + 1, Active: req.Active, Values: req.Values, Rules: req.Rules}, nil
	}}
	h := NewRouter(nil, nil, nil, nil, nil, writer, nil, nil)
	body := `{
		"actor": "tester",
		"expectedRevision": "3",
		"active": true,
		"values": {"nombre": {"kind": "TEXT", "value": "acero"}},
		"rules": []
	}`
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPut, "/v1/catalog/UNIDAD/42", strings.NewReader(body)))

	wantRequest := resourcecore.CatalogUpdateRequest{
		Actor: "tester", Kind: "UNIDAD", ID: 42, ExpectedRevision: 3, Active: true,
		Values: map[string]resourcecore.Value{"nombre": {Kind: resourcecore.ValueText, Text: "acero"}},
		Rules:  []resourcecore.ApplicabilityRule{},
	}
	if !reflect.DeepEqual(captured, wantRequest) {
		t.Fatalf("request = %#v, want %#v", captured, wantRequest)
	}
	if r.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", r.Code, r.Body.String())
	}
	var response struct {
		ID       string `json:"id"`
		Revision string `json:"revision"`
	}
	if err := json.NewDecoder(r.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.ID != "42" || response.Revision != "4" {
		t.Fatalf("response = %#v", response)
	}
}

func TestUpdateCatalogRejectsInvalidIDWithoutCallingCore(t *testing.T) {
	called := false
	writer := catalogWriterFuncs{update: func(context.Context, resourcecore.CatalogUpdateRequest) (resourcecore.CatalogRecord, error) {
		called = true
		return resourcecore.CatalogRecord{}, nil
	}}
	h := NewRouter(nil, nil, nil, nil, nil, writer, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPut, "/v1/catalog/UNIDAD/0", strings.NewReader(`{"actor":"a","expectedRevision":"1","values":{"x":{"kind":"TEXT","value":"y"}}}`)))
	var got errorResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if r.Code != http.StatusBadRequest || got.Error != "invalid request" || called {
		t.Fatalf("status/error/called = %d/%q/%t", r.Code, got.Error, called)
	}
}

func TestUpdateCatalogRejectsInvalidBodyWithoutCallingCore(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"invalid JSON", "{not json"},
		{"missing expected revision", `{"actor":"a","values":{"x":{"kind":"TEXT","value":"y"}}}`},
		{"non-numeric expected revision", `{"actor":"a","expectedRevision":"bad","values":{"x":{"kind":"TEXT","value":"y"}}}`},
		{"unknown value kind", `{"actor":"a","expectedRevision":"1","values":{"x":{"kind":"FUTURE","value":"y"}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			writer := catalogWriterFuncs{update: func(context.Context, resourcecore.CatalogUpdateRequest) (resourcecore.CatalogRecord, error) {
				called = true
				return resourcecore.CatalogRecord{}, nil
			}}
			h := NewRouter(nil, nil, nil, nil, nil, writer, nil, nil)
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodPut, "/v1/catalog/UNIDAD/1", strings.NewReader(tc.body)))
			var got errorResponse
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			if r.Code != http.StatusBadRequest || got.Error != "invalid request" || called {
				t.Fatalf("status/error/called = %d/%q/%t", r.Code, got.Error, called)
			}
		})
	}
}

func TestUpdateCatalogSanitizesNilWriter(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPut, "/v1/catalog/UNIDAD/1", strings.NewReader(`{"actor":"a","expectedRevision":"1","values":{"x":{"kind":"TEXT","value":"y"}}}`)))
	var got errorResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if r.Code != http.StatusInternalServerError || got.Error != "internal server error" {
		t.Fatalf("status/error = %d/%q", r.Code, got.Error)
	}
}

func TestUpdateCatalogMapsAndSanitizesCoreErrors(t *testing.T) {
	for _, tc := range []struct {
		code    resourcecore.ErrorCode
		status  int
		message string
	}{
		{resourcecore.NotFound, 404, "not found"},
		{resourcecore.Conflict, 409, "conflict"},
		{resourcecore.InvalidCatalog, 422, "validation failed"},
	} {
		t.Run(string(tc.code), func(t *testing.T) {
			writer := catalogWriterFuncs{update: func(context.Context, resourcecore.CatalogUpdateRequest) (resourcecore.CatalogRecord, error) {
				return resourcecore.CatalogRecord{}, resourcecore.NewError(tc.code, "postgres://user:secret@host/db")
			}}
			h := NewRouter(nil, nil, nil, nil, nil, writer, nil, nil)
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodPut, "/v1/catalog/UNIDAD/1", strings.NewReader(`{"actor":"a","expectedRevision":"1","values":{"x":{"kind":"TEXT","value":"y"}}}`)))
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

func TestCatalogUpdateRequestOpenAPIShape(t *testing.T) {
	schema := catalogSchema(t, "CatalogUpdateRequest")
	valid := map[string]any{
		"actor": "a", "expectedRevision": "1", "active": true,
		"values": map[string]any{"x": map[string]any{"kind": "TEXT", "value": "y"}},
		"rules":  []any{},
	}
	if err := schema.VisitJSON(valid); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	withExtra := map[string]any{}
	for k, v := range valid {
		withExtra[k] = v
	}
	withExtra["extra"] = true
	if err := schema.VisitJSON(withExtra); err == nil {
		t.Fatal("extra property matched CatalogUpdateRequest")
	}
	for _, key := range []string{"actor", "expectedRevision", "values"} {
		missing := map[string]any{}
		for k, v := range valid {
			if k != key {
				missing[k] = v
			}
		}
		if err := schema.VisitJSON(missing); err == nil {
			t.Fatalf("request without %q matched schema", key)
		}
	}
}
