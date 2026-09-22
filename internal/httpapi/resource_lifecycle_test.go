package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GARFEX33/garfex-backend/resourcecore"
)

func TestResourceLifecycleMapsRequestAndResponse(t *testing.T) {
	for _, tc := range []struct {
		action string
		call   func(*resourceWriterFuncs, func(context.Context, resourcecore.ResourceLifecycleRequest) (resourcecore.Resource, error))
	}{
		{"deactivate", func(w *resourceWriterFuncs, f func(context.Context, resourcecore.ResourceLifecycleRequest) (resourcecore.Resource, error)) {
			w.deactivate = f
		}},
		{"reactivate", func(w *resourceWriterFuncs, f func(context.Context, resourcecore.ResourceLifecycleRequest) (resourcecore.Resource, error)) {
			w.reactivate = f
		}},
	} {
		t.Run(tc.action, func(t *testing.T) {
			var captured resourcecore.ResourceLifecycleRequest
			writer := &resourceWriterFuncs{}
			tc.call(writer, func(_ context.Context, req resourcecore.ResourceLifecycleRequest) (resourcecore.Resource, error) {
				captured = req
				return resourcecore.Resource{ID: req.ID, Revision: req.ExpectedRevision + 1}, nil
			})
			h := NewRouter(nil, nil, writer, nil, nil, nil, nil, nil)
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/resources/42/"+tc.action, strings.NewReader(`{"actor":"tester","expectedRevision":"3"}`)))

			want := resourcecore.ResourceLifecycleRequest{Actor: "tester", ID: 42, ExpectedRevision: 3}
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

func TestResourceLifecycleRejectsInvalidIDWithoutCallingCore(t *testing.T) {
	called := false
	writer := resourceWriterFuncs{deactivate: func(context.Context, resourcecore.ResourceLifecycleRequest) (resourcecore.Resource, error) {
		called = true
		return resourcecore.Resource{}, nil
	}}
	h := NewRouter(nil, nil, writer, nil, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/resources/0/deactivate", strings.NewReader(`{"actor":"a","expectedRevision":"1"}`)))
	var got errorResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if r.Code != http.StatusBadRequest || got.Error != "invalid request" || called {
		t.Fatalf("status/error/called = %d/%q/%t", r.Code, got.Error, called)
	}
}

func TestResourceLifecycleRejectsInvalidBodyWithoutCallingCore(t *testing.T) {
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
			writer := resourceWriterFuncs{deactivate: func(context.Context, resourcecore.ResourceLifecycleRequest) (resourcecore.Resource, error) {
				called = true
				return resourcecore.Resource{}, nil
			}}
			h := NewRouter(nil, nil, writer, nil, nil, nil, nil, nil)
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/resources/1/deactivate", strings.NewReader(tc.body)))
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

func TestResourceLifecycleSanitizesNilWriter(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/resources/1/deactivate", strings.NewReader(`{"actor":"a","expectedRevision":"1"}`)))
	var got errorResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if r.Code != http.StatusInternalServerError || got.Error != "internal server error" {
		t.Fatalf("status/error = %d/%q", r.Code, got.Error)
	}
}

func TestResourceLifecycleMapsAndSanitizesCoreErrors(t *testing.T) {
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
			writer := resourceWriterFuncs{deactivate: func(context.Context, resourcecore.ResourceLifecycleRequest) (resourcecore.Resource, error) {
				return resourcecore.Resource{}, resourcecore.NewError(tc.code, "postgres://user:secret@host/db")
			}}
			h := NewRouter(nil, nil, writer, nil, nil, nil, nil, nil)
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/resources/1/deactivate", strings.NewReader(`{"actor":"a","expectedRevision":"1"}`)))
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

func TestResourceLifecycleRejectsWrongMethod(t *testing.T) {
	h := NewRouter(nil, nil, resourceWriterFuncs{}, nil, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/resources/1/deactivate", nil))
	if r.Code != http.StatusMethodNotAllowed || r.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("status/allow = %d/%q", r.Code, r.Header().Get("Allow"))
	}
}

func TestResourceLifecycleRequestOpenAPIShape(t *testing.T) {
	schema := catalogSchema(t, "ResourceLifecycleRequest")
	if err := schema.VisitJSON(map[string]any{"actor": "a", "expectedRevision": "1"}); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	if err := schema.VisitJSON(map[string]any{"actor": "a", "expectedRevision": "1", "extra": true}); err == nil {
		t.Fatal("extra property matched ResourceLifecycleRequest")
	}
	if err := schema.VisitJSON(map[string]any{"expectedRevision": "1"}); err == nil {
		t.Fatal("ResourceLifecycleRequest matched without actor")
	}
	if err := schema.VisitJSON(map[string]any{"actor": "a"}); err == nil {
		t.Fatal("ResourceLifecycleRequest matched without expectedRevision")
	}
}

func TestResourceLifecyclePathRejectsUnknownAction(t *testing.T) {
	// An unreserved second segment falls through to the natural-key detail
	// route, which only accepts GET.
	h := NewRouter(nil, nil, resourceWriterFuncs{}, nil, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/resources/1/archive", nil))
	if r.Code != http.StatusMethodNotAllowed || r.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("status/allow = %d/%q", r.Code, r.Header().Get("Allow"))
	}
}
