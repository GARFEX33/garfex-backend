package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/GARFEX33/garfex-costos-unitarios/resourcecore"
)

type catalogWriterFuncs struct {
	create     func(context.Context, resourcecore.CatalogWriteRequest) (resourcecore.CatalogRecord, error)
	update     func(context.Context, resourcecore.CatalogUpdateRequest) (resourcecore.CatalogRecord, error)
	deactivate func(context.Context, resourcecore.CatalogLifecycleRequest) (resourcecore.CatalogRecord, error)
	reactivate func(context.Context, resourcecore.CatalogLifecycleRequest) (resourcecore.CatalogRecord, error)
	hardDelete func(context.Context, resourcecore.CatalogLifecycleRequest) error
}

func (f catalogWriterFuncs) CreateCatalog(ctx context.Context, req resourcecore.CatalogWriteRequest) (resourcecore.CatalogRecord, error) {
	return f.create(ctx, req)
}

func (f catalogWriterFuncs) UpdateCatalog(ctx context.Context, req resourcecore.CatalogUpdateRequest) (resourcecore.CatalogRecord, error) {
	return f.update(ctx, req)
}

func (f catalogWriterFuncs) DeactivateCatalog(ctx context.Context, req resourcecore.CatalogLifecycleRequest) (resourcecore.CatalogRecord, error) {
	return f.deactivate(ctx, req)
}

func (f catalogWriterFuncs) HardDeleteCatalog(ctx context.Context, req resourcecore.CatalogLifecycleRequest) error {
	return f.hardDelete(ctx, req)
}

func (f catalogWriterFuncs) ReactivateCatalog(ctx context.Context, req resourcecore.CatalogLifecycleRequest) (resourcecore.CatalogRecord, error) {
	return f.reactivate(ctx, req)
}

func TestCreateCatalogMapsRequestAndResponse(t *testing.T) {
	var captured resourcecore.CatalogWriteRequest
	writer := catalogWriterFuncs{create: func(_ context.Context, req resourcecore.CatalogWriteRequest) (resourcecore.CatalogRecord, error) {
		captured = req
		return resourcecore.CatalogRecord{
			Kind: req.Kind, ID: 9007199254740993, Revision: 1, Active: req.Active,
			Values: req.Values, Rules: req.Rules,
		}, nil
	}}
	h := NewRouter(nil, nil, nil, nil, nil, writer)
	body := `{
		"actor": "tester",
		"active": true,
		"values": {"nombre": {"kind": "TEXT", "value": "acero"}},
		"rules": [{"attributeCode": "acabado", "equals": {"kind": "TEXT", "value": "pulido"}, "mode": "REQUIRED", "identityParticipates": true, "notApplicable": false, "active": true}]
	}`
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/catalog/UNIDAD", strings.NewReader(body)))

	wantRequest := resourcecore.CatalogWriteRequest{
		Actor: "tester", Kind: "UNIDAD", Active: true,
		Values: map[string]resourcecore.Value{"nombre": {Kind: resourcecore.ValueText, Text: "acero"}},
		Rules: []resourcecore.ApplicabilityRule{{
			AttributeCode: "acabado", Equals: resourcecore.Value{Kind: resourcecore.ValueText, Text: "pulido"},
			Mode: "REQUIRED", IdentityParticipates: true, NotApplicable: false, Active: true,
		}},
	}
	if !reflect.DeepEqual(captured, wantRequest) {
		t.Fatalf("request = %#v, want %#v", captured, wantRequest)
	}
	if r.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", r.Code, r.Body.String())
	}
	var input any
	if err := json.Unmarshal(r.Body.Bytes(), &input); err != nil {
		t.Fatal(err)
	}
	if err := catalogSchema(t, "CatalogRecord").VisitJSON(input); err != nil {
		t.Fatalf("CatalogRecord schema validation failed: %v", err)
	}
}

func TestCreateCatalogPreservesNilVersusEmptyRules(t *testing.T) {
	for _, tc := range []struct {
		name      string
		body      string
		wantRules []resourcecore.ApplicabilityRule
	}{
		{"omitted rules stays nil", `{"actor":"a","values":{"x":{"kind":"TEXT","value":"y"}}}`, nil},
		{"explicit empty rules stays non-nil empty", `{"actor":"a","values":{"x":{"kind":"TEXT","value":"y"}},"rules":[]}`, []resourcecore.ApplicabilityRule{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var captured resourcecore.CatalogWriteRequest
			writer := catalogWriterFuncs{create: func(_ context.Context, req resourcecore.CatalogWriteRequest) (resourcecore.CatalogRecord, error) {
				captured = req
				return resourcecore.CatalogRecord{Kind: req.Kind, ID: 1, Revision: 1, Values: req.Values, Rules: req.Rules}, nil
			}}
			h := NewRouter(nil, nil, nil, nil, nil, writer)
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/catalog/UNIDAD", strings.NewReader(tc.body)))
			if r.Code != http.StatusCreated {
				t.Fatalf("status = %d, body = %s", r.Code, r.Body.String())
			}
			if (captured.Rules == nil) != (tc.wantRules == nil) || len(captured.Rules) != len(tc.wantRules) {
				t.Fatalf("rules = %#v, want %#v", captured.Rules, tc.wantRules)
			}
		})
	}
}

func TestCreateCatalogRejectsInvalidBodyWithoutCallingCore(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"invalid JSON", "{not json"},
		{"unknown value kind in values", `{"actor":"a","values":{"x":{"kind":"FUTURE","value":"y"}}}`},
		{"unknown value kind in rule equals", `{"actor":"a","values":{"x":{"kind":"TEXT","value":"y"}},"rules":[{"attributeCode":"a","equals":{"kind":"FUTURE","value":"y"},"mode":"REQUIRED"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			writer := catalogWriterFuncs{create: func(context.Context, resourcecore.CatalogWriteRequest) (resourcecore.CatalogRecord, error) {
				called = true
				return resourcecore.CatalogRecord{}, nil
			}}
			h := NewRouter(nil, nil, nil, nil, nil, writer)
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/catalog/UNIDAD", strings.NewReader(tc.body)))
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

func TestCreateCatalogSanitizesNilWriter(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/catalog/UNIDAD", strings.NewReader(`{"actor":"a","values":{"x":{"kind":"TEXT","value":"y"}}}`)))
	var got errorResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if r.Code != http.StatusInternalServerError || got.Error != "internal server error" {
		t.Fatalf("status/error = %d/%q", r.Code, got.Error)
	}
}

func TestCreateCatalogMapsAndSanitizesCoreErrors(t *testing.T) {
	for _, tc := range []struct {
		code    resourcecore.ErrorCode
		status  int
		message string
	}{
		{resourcecore.InvalidArgument, 400, "invalid request"},
		{resourcecore.Duplicate, 409, "conflict"},
		{resourcecore.InvalidCatalog, 422, "validation failed"},
	} {
		t.Run(string(tc.code), func(t *testing.T) {
			writer := catalogWriterFuncs{create: func(context.Context, resourcecore.CatalogWriteRequest) (resourcecore.CatalogRecord, error) {
				return resourcecore.CatalogRecord{}, resourcecore.NewError(tc.code, "postgres://user:secret@host/db")
			}}
			h := NewRouter(nil, nil, nil, nil, nil, writer)
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/catalog/UNIDAD", strings.NewReader(`{"actor":"a","values":{"x":{"kind":"TEXT","value":"y"}}}`)))
			var got errorResponse
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			if r.Code != tc.status || got.Error != tc.message {
				t.Fatalf("status/error = %d/%q, want %d/%q", r.Code, got.Error, tc.status, tc.message)
			}
		})
	}
	if status, _ := catalogError(errors.New("secret")); status != http.StatusInternalServerError {
		t.Fatalf("unknown error status = %d, want 500", status)
	}
}

func TestCatalogByKindRejectsUnsupportedMethod(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPatch, "/v1/catalog/UNIDAD", nil))
	var got errorResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if r.Code != http.StatusMethodNotAllowed || r.Header().Get("Allow") != "GET, POST" || got.Error != "method not allowed" {
		t.Fatalf("status/allow/error = %d/%q/%q", r.Code, r.Header().Get("Allow"), got.Error)
	}
}

func TestCatalogCreateRequestOpenAPIShape(t *testing.T) {
	schema := catalogSchema(t, "CatalogCreateRequest")
	valid := map[string]any{
		"actor": "a", "active": true,
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
		t.Fatal("extra property matched CatalogCreateRequest")
	}
	for _, key := range []string{"actor", "values"} {
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
