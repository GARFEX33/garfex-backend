package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GARFEX33/garfex-costos-unitarios/resourcecore"
)

func TestCatalogLifecycleMapsRequestAndResponse(t *testing.T) {
	for _, tc := range []struct {
		action string
		call   func(*catalogWriterFuncs, func(context.Context, resourcecore.CatalogLifecycleRequest) (resourcecore.CatalogRecord, error))
	}{
		{"deactivate", func(w *catalogWriterFuncs, f func(context.Context, resourcecore.CatalogLifecycleRequest) (resourcecore.CatalogRecord, error)) {
			w.deactivate = f
		}},
		{"reactivate", func(w *catalogWriterFuncs, f func(context.Context, resourcecore.CatalogLifecycleRequest) (resourcecore.CatalogRecord, error)) {
			w.reactivate = f
		}},
	} {
		t.Run(tc.action, func(t *testing.T) {
			var captured resourcecore.CatalogLifecycleRequest
			writer := &catalogWriterFuncs{}
			tc.call(writer, func(_ context.Context, req resourcecore.CatalogLifecycleRequest) (resourcecore.CatalogRecord, error) {
				captured = req
				return resourcecore.CatalogRecord{Kind: req.Kind, ID: req.ID, Revision: req.ExpectedRevision + 1}, nil
			})
			h := NewRouter(nil, nil, nil, nil, nil, writer)
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/catalog/UNIDAD/42/"+tc.action, strings.NewReader(`{"actor":"tester","expectedRevision":"3"}`)))

			want := resourcecore.CatalogLifecycleRequest{Actor: "tester", Kind: "UNIDAD", ID: 42, ExpectedRevision: 3}
			if captured != want {
				t.Fatalf("request = %#v, want %#v", captured, want)
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
		})
	}
}

func TestCatalogLifecycleRejectsInvalidIDWithoutCallingCore(t *testing.T) {
	called := false
	writer := catalogWriterFuncs{deactivate: func(context.Context, resourcecore.CatalogLifecycleRequest) (resourcecore.CatalogRecord, error) {
		called = true
		return resourcecore.CatalogRecord{}, nil
	}}
	h := NewRouter(nil, nil, nil, nil, nil, writer)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/catalog/UNIDAD/0/deactivate", strings.NewReader(`{"actor":"a","expectedRevision":"1"}`)))
	var got errorResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if r.Code != http.StatusBadRequest || got.Error != "invalid request" || called {
		t.Fatalf("status/error/called = %d/%q/%t", r.Code, got.Error, called)
	}
}

func TestCatalogLifecycleRejectsInvalidBodyWithoutCallingCore(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"invalid JSON", "{not json"},
		{"missing expected revision", `{"actor":"a"}`},
		{"non-numeric expected revision", `{"actor":"a","expectedRevision":"bad"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			writer := catalogWriterFuncs{deactivate: func(context.Context, resourcecore.CatalogLifecycleRequest) (resourcecore.CatalogRecord, error) {
				called = true
				return resourcecore.CatalogRecord{}, nil
			}}
			h := NewRouter(nil, nil, nil, nil, nil, writer)
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/catalog/UNIDAD/1/deactivate", strings.NewReader(tc.body)))
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

func TestCatalogLifecycleSanitizesNilWriter(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/catalog/UNIDAD/1/deactivate", strings.NewReader(`{"actor":"a","expectedRevision":"1"}`)))
	var got errorResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if r.Code != http.StatusInternalServerError || got.Error != "internal server error" {
		t.Fatalf("status/error = %d/%q", r.Code, got.Error)
	}
}

func TestCatalogLifecycleMapsAndSanitizesCoreErrors(t *testing.T) {
	for _, tc := range []struct {
		code    resourcecore.ErrorCode
		status  int
		message string
	}{
		{resourcecore.NotFound, 404, "not found"},
		{resourcecore.InvalidLifecycle, 409, "conflict"},
		{resourcecore.ReactivationImpossible, 409, "conflict"},
	} {
		t.Run(string(tc.code), func(t *testing.T) {
			writer := catalogWriterFuncs{deactivate: func(context.Context, resourcecore.CatalogLifecycleRequest) (resourcecore.CatalogRecord, error) {
				return resourcecore.CatalogRecord{}, resourcecore.NewError(tc.code, "postgres://user:secret@host/db")
			}}
			h := NewRouter(nil, nil, nil, nil, nil, writer)
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/catalog/UNIDAD/1/deactivate", strings.NewReader(`{"actor":"a","expectedRevision":"1"}`)))
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

func TestCatalogLifecycleRejectsWrongMethod(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, catalogWriterFuncs{})
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/catalog/UNIDAD/1/deactivate", nil))
	if r.Code != http.StatusMethodNotAllowed || r.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("status/allow = %d/%q", r.Code, r.Header().Get("Allow"))
	}
}

func TestCatalogLifecyclePathRejectsUnknownAction(t *testing.T) {
	// An unreserved third segment falls through to 404: catalogDetailPath's
	// two-segment Cut rejects it (its id part would contain a "/").
	h := NewRouter(nil, nil, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/catalog/UNIDAD/1/archive", nil))
	if r.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", r.Code)
	}
}

func TestCatalogLifecycleRequestOpenAPIShape(t *testing.T) {
	schema := catalogSchema(t, "CatalogLifecycleRequest")
	if err := schema.VisitJSON(map[string]any{"actor": "a", "expectedRevision": "1"}); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	if err := schema.VisitJSON(map[string]any{"actor": "a", "expectedRevision": "1", "extra": true}); err == nil {
		t.Fatal("extra property matched CatalogLifecycleRequest")
	}
	if err := schema.VisitJSON(map[string]any{"expectedRevision": "1"}); err == nil {
		t.Fatal("CatalogLifecycleRequest matched without actor")
	}
	if err := schema.VisitJSON(map[string]any{"actor": "a"}); err == nil {
		t.Fatal("CatalogLifecycleRequest matched without expectedRevision")
	}
}
