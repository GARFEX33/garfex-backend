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

type supplierReaderFuncs struct {
	get          func(context.Context, int64) (suppliercore.Supplier, error)
	search       func(context.Context, suppliercore.SupplierQuery) (suppliercore.SupplierPage, error)
	listBranches func(context.Context, suppliercore.BranchQuery) (suppliercore.BranchPage, error)
	getBranch    func(context.Context, suppliercore.BranchKey) (suppliercore.Branch, error)
	listContacts func(context.Context, suppliercore.ContactQuery) (suppliercore.ContactPage, error)
	getContact   func(context.Context, suppliercore.ContactKey) (suppliercore.Contact, error)
}

func (f supplierReaderFuncs) GetSupplier(ctx context.Context, id int64) (suppliercore.Supplier, error) {
	return f.get(ctx, id)
}

func (f supplierReaderFuncs) SearchSuppliers(ctx context.Context, q suppliercore.SupplierQuery) (suppliercore.SupplierPage, error) {
	return f.search(ctx, q)
}

func (f supplierReaderFuncs) ListBranches(ctx context.Context, q suppliercore.BranchQuery) (suppliercore.BranchPage, error) {
	return f.listBranches(ctx, q)
}

func (f supplierReaderFuncs) GetBranch(ctx context.Context, key suppliercore.BranchKey) (suppliercore.Branch, error) {
	return f.getBranch(ctx, key)
}

func (f supplierReaderFuncs) ListContacts(ctx context.Context, q suppliercore.ContactQuery) (suppliercore.ContactPage, error) {
	return f.listContacts(ctx, q)
}

func (f supplierReaderFuncs) GetContact(ctx context.Context, key suppliercore.ContactKey) (suppliercore.Contact, error) {
	return f.getContact(ctx, key)
}

func TestSupplierSearchPassesQueryAndMapsPage(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	var captured suppliercore.SupplierQuery
	reader := supplierReaderFuncs{search: func(_ context.Context, q suppliercore.SupplierQuery) (suppliercore.SupplierPage, error) {
		captured = q
		return suppliercore.SupplierPage{
			Suppliers:   []suppliercore.Supplier{{ID: 1, TradeName: "Acme", CreatedAt: created, UpdatedAt: created}},
			HasPrevious: true, HasNext: true,
		}, nil
	}}
	h := NewRouter(nil, nil, nil, reader, nil, nil)

	for _, tc := range []struct {
		path string
		want suppliercore.SupplierQuery
	}{
		{"/v1/suppliers", suppliercore.SupplierQuery{Scope: suppliercore.ScopeActive, Limit: 50}},
		{"/v1/suppliers?scope=ACTIVE&text=acme&limit=1&offset=2", suppliercore.SupplierQuery{Scope: suppliercore.ScopeActive, Text: "acme", Limit: 1, Offset: 2}},
		{"/v1/suppliers?scope=ALL", suppliercore.SupplierQuery{Scope: suppliercore.ScopeAll, Limit: 50}},
	} {
		t.Run(tc.path, func(t *testing.T) {
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if r.Code != http.StatusOK || !reflect.DeepEqual(captured, tc.want) {
				t.Fatalf("status/query = %d/%#v, want 200/%#v", r.Code, captured, tc.want)
			}
			var page supplierPageResponse
			if err := json.NewDecoder(r.Body).Decode(&page); err != nil {
				t.Fatal(err)
			}
			if len(page.Suppliers) != 1 || !page.HasPrevious || !page.HasNext {
				t.Fatalf("page = %#v", page)
			}
			validateSchemaJSON(t, catalogSchema(t, "SupplierPage"), page)
		})
	}
}

func TestSupplierSearchRejectsInvalidParametersWithoutCallingCore(t *testing.T) {
	called := false
	reader := supplierReaderFuncs{search: func(context.Context, suppliercore.SupplierQuery) (suppliercore.SupplierPage, error) {
		called = true
		return suppliercore.SupplierPage{}, nil
	}}
	h := NewRouter(nil, nil, nil, reader, nil, nil)
	for _, query := range []string{"limit=0", "limit=51", "limit=bad", "offset=-1", "offset=bad", "scope=RETIRED"} {
		t.Run(query, func(t *testing.T) {
			called = false
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/suppliers?"+query, nil))
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

func TestSupplierSearchSanitizesNilReader(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/suppliers", nil))
	var got errorResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if r.Code != http.StatusInternalServerError || got.Error != "internal server error" {
		t.Fatalf("status/error = %d/%q", r.Code, got.Error)
	}
}

func TestSupplierDetailMapsRecordAndSanitizesFailures(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	cases := []struct {
		name    string
		reader  SupplierReader
		id      string
		status  int
		message string
	}{
		{"happy path", supplierReaderFuncs{get: func(_ context.Context, id int64) (suppliercore.Supplier, error) {
			if id != 9007199254740993 {
				t.Fatalf("id = %d", id)
			}
			return suppliercore.Supplier{ID: id, TradeName: "Acme", Active: true, CreatedAt: created, UpdatedAt: created}, nil
		}}, "9007199254740993", http.StatusOK, ""},
		{"invalid id", supplierReaderFuncs{}, "0", http.StatusBadRequest, "invalid request"},
		{"nil reader", nil, "1", http.StatusInternalServerError, "internal server error"},
		{"not found", supplierReaderFuncs{get: func(context.Context, int64) (suppliercore.Supplier, error) {
			return suppliercore.Supplier{}, suppliercore.NewError(suppliercore.NotFound, "postgres://user:secret@host/db")
		}}, "1", http.StatusNotFound, "not found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewRouter(nil, nil, nil, tc.reader, nil, nil)
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/suppliers/"+tc.id, nil))
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

func TestSupplierDetailRejectsWrongMethod(t *testing.T) {
	h := NewRouter(nil, nil, nil, supplierReaderFuncs{}, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/suppliers/1", nil))
	if r.Code != http.StatusMethodNotAllowed || r.Header().Get("Allow") != "GET, PUT" {
		t.Fatalf("status/allow = %d/%q", r.Code, r.Header().Get("Allow"))
	}
}
