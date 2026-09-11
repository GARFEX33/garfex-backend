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

func TestHardDeleteCatalogMapsRequest(t *testing.T) {
	var captured resourcecore.CatalogLifecycleRequest
	writer := catalogWriterFuncs{hardDelete: func(_ context.Context, req resourcecore.CatalogLifecycleRequest) error {
		captured = req
		return nil
	}}
	h := NewRouter(nil, nil, nil, nil, nil, writer)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodDelete, "/v1/catalog/UNIDAD/42", strings.NewReader(`{"actor":"tester","expectedRevision":"3"}`)))

	want := resourcecore.CatalogLifecycleRequest{Actor: "tester", Kind: "UNIDAD", ID: 42, ExpectedRevision: 3}
	if captured != want {
		t.Fatalf("request = %#v, want %#v", captured, want)
	}
	if r.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", r.Code, r.Body.String())
	}
	if r.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", r.Body.String())
	}
}

func TestHardDeleteCatalogRejectsInvalidIDWithoutCallingCore(t *testing.T) {
	called := false
	writer := catalogWriterFuncs{hardDelete: func(context.Context, resourcecore.CatalogLifecycleRequest) error {
		called = true
		return nil
	}}
	h := NewRouter(nil, nil, nil, nil, nil, writer)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodDelete, "/v1/catalog/UNIDAD/0", strings.NewReader(`{"actor":"a","expectedRevision":"1"}`)))
	var got errorResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if r.Code != http.StatusBadRequest || got.Error != "invalid request" || called {
		t.Fatalf("status/error/called = %d/%q/%t", r.Code, got.Error, called)
	}
}

func TestHardDeleteCatalogRejectsInvalidBodyWithoutCallingCore(t *testing.T) {
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
			writer := catalogWriterFuncs{hardDelete: func(context.Context, resourcecore.CatalogLifecycleRequest) error {
				called = true
				return nil
			}}
			h := NewRouter(nil, nil, nil, nil, nil, writer)
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodDelete, "/v1/catalog/UNIDAD/1", strings.NewReader(tc.body)))
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

func TestHardDeleteCatalogSanitizesNilWriter(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodDelete, "/v1/catalog/UNIDAD/1", strings.NewReader(`{"actor":"a","expectedRevision":"1"}`)))
	var got errorResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if r.Code != http.StatusInternalServerError || got.Error != "internal server error" {
		t.Fatalf("status/error = %d/%q", r.Code, got.Error)
	}
}

func TestHardDeleteCatalogMapsAndSanitizesCoreErrors(t *testing.T) {
	for _, tc := range []struct {
		code    resourcecore.ErrorCode
		status  int
		message string
	}{
		{resourcecore.NotFound, 404, "not found"},
		{resourcecore.Conflict, 409, "conflict"},
		{resourcecore.InUse, 409, "conflict"},
	} {
		t.Run(string(tc.code), func(t *testing.T) {
			writer := catalogWriterFuncs{hardDelete: func(context.Context, resourcecore.CatalogLifecycleRequest) error {
				return resourcecore.NewError(tc.code, "postgres://user:secret@host/db")
			}}
			h := NewRouter(nil, nil, nil, nil, nil, writer)
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodDelete, "/v1/catalog/UNIDAD/1", strings.NewReader(`{"actor":"a","expectedRevision":"1"}`)))
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
