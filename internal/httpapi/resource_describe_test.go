package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GARFEX33/garfex-costos-unitarios/resourcecore"
)

func TestResourceDescribeMapsResponse(t *testing.T) {
	reader := resourceReaderFuncs{describe: func(_ context.Context, key resourcecore.ResourceKey) (string, error) {
		if key != (resourcecore.ResourceKey{ClassCode: "MATERIAL", IdentityV1: "abc"}) {
			t.Fatalf("key = %#v", key)
		}
		return "Acero 1/2\" x 6m", nil
	}}
	h := NewRouter(nil, nil, nil, nil, reader, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/resources/MATERIAL/abc/describe", nil))
	if r.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", r.Code, r.Body.String())
	}
	var response resourceDescriptionResponse
	if err := json.NewDecoder(r.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Description != "Acero 1/2\" x 6m" {
		t.Fatalf("response = %#v", response)
	}
}

func TestResourceDescribeSanitizesFailures(t *testing.T) {
	cases := []struct {
		name    string
		reader  ResourceReader
		status  int
		message string
	}{
		{"nil reader", nil, http.StatusInternalServerError, "internal server error"},
		{"not found", resourceReaderFuncs{describe: func(context.Context, resourcecore.ResourceKey) (string, error) {
			return "", resourcecore.NewError(resourcecore.NotFound, "postgres://user:secret@host/db")
		}}, http.StatusNotFound, "not found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewRouter(nil, nil, nil, nil, tc.reader, nil, nil, nil)
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/resources/MATERIAL/abc/describe", nil))
			if r.Code != tc.status {
				t.Fatalf("status = %d, want %d, body = %s", r.Code, tc.status, r.Body.String())
			}
			var got errorResponse
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil || got.Error != tc.message {
				t.Fatalf("error = %q, want %q, decode error = %v", got.Error, tc.message, err)
			}
		})
	}
}

func TestResourceDescribeRejectsWrongMethod(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, resourceReaderFuncs{}, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/resources/MATERIAL/abc/describe", nil))
	if r.Code != http.StatusMethodNotAllowed || r.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("status/allow = %d/%q", r.Code, r.Header().Get("Allow"))
	}
}
