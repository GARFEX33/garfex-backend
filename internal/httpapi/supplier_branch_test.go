package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/GARFEX33/garfex-costos-unitarios/suppliercore"
)

func TestBranchListPassesQueryAndMapsPage(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	var captured suppliercore.BranchQuery
	reader := supplierReaderFuncs{listBranches: func(_ context.Context, q suppliercore.BranchQuery) (suppliercore.BranchPage, error) {
		captured = q
		return suppliercore.BranchPage{
			Branches:    []suppliercore.Branch{{ID: 1, SupplierID: 42, Name: "Main", CreatedAt: created, UpdatedAt: created}},
			HasPrevious: true, HasNext: true,
		}, nil
	}}
	h := NewRouter(nil, nil, nil, reader, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/suppliers/42/branches?text=main&limit=5&offset=1", nil))

	want := suppliercore.BranchQuery{SupplierID: 42, Scope: suppliercore.ScopeActive, Text: "main", Limit: 5, Offset: 1}
	if r.Code != http.StatusOK || !reflect.DeepEqual(captured, want) {
		t.Fatalf("status/query = %d/%#v, want 200/%#v", r.Code, captured, want)
	}
	var page branchPageResponse
	if err := json.NewDecoder(r.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if len(page.Branches) != 1 || page.Branches[0].SupplierID != "42" || !page.HasPrevious || !page.HasNext {
		t.Fatalf("page = %#v", page)
	}
}

func TestBranchListRejectsInvalidSupplierIDWithoutCallingCore(t *testing.T) {
	called := false
	reader := supplierReaderFuncs{listBranches: func(context.Context, suppliercore.BranchQuery) (suppliercore.BranchPage, error) {
		called = true
		return suppliercore.BranchPage{}, nil
	}}
	h := NewRouter(nil, nil, nil, reader, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/suppliers/0/branches", nil))
	var got errorResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if r.Code != http.StatusBadRequest || got.Error != "invalid request" || called {
		t.Fatalf("status/error/called = %d/%q/%t", r.Code, got.Error, called)
	}
}

func TestBranchListSanitizesNilReader(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/suppliers/1/branches", nil))
	var got errorResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if r.Code != http.StatusInternalServerError || got.Error != "internal server error" {
		t.Fatalf("status/error = %d/%q", r.Code, got.Error)
	}
}

func TestBranchListRejectsWrongMethod(t *testing.T) {
	h := NewRouter(nil, nil, nil, supplierReaderFuncs{}, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/suppliers/1/branches", nil))
	if r.Code != http.StatusMethodNotAllowed || r.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("status/allow = %d/%q", r.Code, r.Header().Get("Allow"))
	}
}

func TestBranchDetailMapsRecordAndSanitizesFailures(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	cases := []struct {
		name    string
		reader  SupplierReader
		path    string
		status  int
		message string
	}{
		{"happy path", supplierReaderFuncs{getBranch: func(_ context.Context, key suppliercore.BranchKey) (suppliercore.Branch, error) {
			if key != (suppliercore.BranchKey{SupplierID: 42, BranchID: 7}) {
				t.Fatalf("key = %#v", key)
			}
			return suppliercore.Branch{ID: 7, SupplierID: 42, Name: "Main", Active: true, CreatedAt: created, UpdatedAt: created}, nil
		}}, "/v1/suppliers/42/branches/7", http.StatusOK, ""},
		{"invalid branch id", supplierReaderFuncs{}, "/v1/suppliers/42/branches/0", http.StatusBadRequest, "invalid request"},
		{"nil reader", nil, "/v1/suppliers/42/branches/7", http.StatusInternalServerError, "internal server error"},
		{"not found", supplierReaderFuncs{getBranch: func(context.Context, suppliercore.BranchKey) (suppliercore.Branch, error) {
			return suppliercore.Branch{}, suppliercore.NewError(suppliercore.NotFound, "postgres://user:secret@host/db")
		}}, "/v1/suppliers/42/branches/7", http.StatusNotFound, "not found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewRouter(nil, nil, nil, tc.reader, nil, nil)
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if r.Code != tc.status {
				t.Fatalf("status = %d, want %d, body = %s", r.Code, tc.status, r.Body.String())
			}
			if tc.message != "" {
				var got errorResponse
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil || got.Error != tc.message {
					t.Fatalf("error = %q, want %q, decode error = %v", got.Error, tc.message, err)
				}
			}
		})
	}
}

func TestBranchDetailRejectsWrongMethod(t *testing.T) {
	h := NewRouter(nil, nil, nil, supplierReaderFuncs{}, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/suppliers/1/branches/1", nil))
	if r.Code != http.StatusMethodNotAllowed || r.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("status/allow = %d/%q", r.Code, r.Header().Get("Allow"))
	}
}
