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

func TestContactListPassesQueryAndMapsPage(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	var captured suppliercore.ContactQuery
	reader := supplierReaderFuncs{listContacts: func(_ context.Context, q suppliercore.ContactQuery) (suppliercore.ContactPage, error) {
		captured = q
		branchID := int64(7)
		return suppliercore.ContactPage{
			Contacts:    []suppliercore.Contact{{ID: 1, SupplierID: 42, BranchID: &branchID, Name: "Ana", CreatedAt: created, UpdatedAt: created}},
			HasPrevious: true, HasNext: true,
		}, nil
	}}
	h := NewRouter(nil, nil, nil, reader, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/suppliers/42/contacts?text=ana&branchId=7&limit=5&offset=1", nil))

	wantBranchID := int64(7)
	want := suppliercore.ContactQuery{SupplierID: 42, Scope: suppliercore.ScopeActive, Text: "ana", BranchID: &wantBranchID, Limit: 5, Offset: 1}
	if r.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", r.Code, r.Body.String())
	}
	if captured.SupplierID != want.SupplierID || captured.Scope != want.Scope || captured.Text != want.Text ||
		captured.Limit != want.Limit || captured.Offset != want.Offset ||
		captured.BranchID == nil || *captured.BranchID != *want.BranchID {
		t.Fatalf("query = %#v, want %#v", captured, want)
	}
	var page contactPageResponse
	if err := json.NewDecoder(r.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if len(page.Contacts) != 1 || page.Contacts[0].SupplierID != "42" || page.Contacts[0].BranchID != "7" || !page.HasPrevious || !page.HasNext {
		t.Fatalf("page = %#v", page)
	}
}

func TestContactListRejectsInvalidParametersWithoutCallingCore(t *testing.T) {
	called := false
	reader := supplierReaderFuncs{listContacts: func(context.Context, suppliercore.ContactQuery) (suppliercore.ContactPage, error) {
		called = true
		return suppliercore.ContactPage{}, nil
	}}
	h := NewRouter(nil, nil, nil, reader, nil, nil, nil, nil)
	for _, tc := range []struct{ path string }{
		{"/v1/suppliers/0/contacts"},
		{"/v1/suppliers/1/contacts?branchId=0"},
		{"/v1/suppliers/1/contacts?limit=0"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			called = false
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, tc.path, nil))
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

func TestContactListSanitizesNilReader(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/suppliers/1/contacts", nil))
	var got errorResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if r.Code != http.StatusInternalServerError || got.Error != "internal server error" {
		t.Fatalf("status/error = %d/%q", r.Code, got.Error)
	}
}

func TestContactDetailMapsRecordAndOmitsEmptyBranch(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	reader := supplierReaderFuncs{getContact: func(_ context.Context, key suppliercore.ContactKey) (suppliercore.Contact, error) {
		if key != (suppliercore.ContactKey{SupplierID: 42, ContactID: 9}) {
			t.Fatalf("key = %#v", key)
		}
		return suppliercore.Contact{ID: 9, SupplierID: 42, Name: "Ana", Active: true, CreatedAt: created, UpdatedAt: created}, nil
	}}
	h := NewRouter(nil, nil, nil, reader, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/suppliers/42/contacts/9", nil))
	if r.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", r.Code, r.Body.String())
	}
	if body := r.Body.String(); strings.Contains(body, `"branchId"`) {
		t.Fatalf("body unexpectedly carries branchId: %s", body)
	}
}

func TestContactDetailSanitizesFailures(t *testing.T) {
	cases := []struct {
		name    string
		reader  SupplierReader
		path    string
		status  int
		message string
	}{
		{"invalid contact id", supplierReaderFuncs{}, "/v1/suppliers/42/contacts/0", http.StatusBadRequest, "invalid request"},
		{"nil reader", nil, "/v1/suppliers/42/contacts/9", http.StatusInternalServerError, "internal server error"},
		{"not found", supplierReaderFuncs{getContact: func(context.Context, suppliercore.ContactKey) (suppliercore.Contact, error) {
			return suppliercore.Contact{}, suppliercore.NewError(suppliercore.NotFound, "postgres://user:secret@host/db")
		}}, "/v1/suppliers/42/contacts/9", http.StatusNotFound, "not found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewRouter(nil, nil, nil, tc.reader, nil, nil, nil, nil)
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, tc.path, nil))
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

func TestContactListRejectsWrongMethod(t *testing.T) {
	h := NewRouter(nil, nil, nil, supplierReaderFuncs{}, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/suppliers/1/contacts", nil))
	if r.Code != http.StatusMethodNotAllowed || r.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("status/allow = %d/%q", r.Code, r.Header().Get("Allow"))
	}
}

func TestContactDetailRejectsWrongMethod(t *testing.T) {
	h := NewRouter(nil, nil, nil, supplierReaderFuncs{}, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/suppliers/1/contacts/1", nil))
	if r.Code != http.StatusMethodNotAllowed || r.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("status/allow = %d/%q", r.Code, r.Header().Get("Allow"))
	}
}
