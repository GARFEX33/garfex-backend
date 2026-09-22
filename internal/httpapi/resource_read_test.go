package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/GARFEX33/garfex-backend/resourcecore"
)

type resourceReaderFuncs struct {
	get       func(context.Context, resourcecore.ResourceKey) (resourcecore.Resource, error)
	search    func(context.Context, resourcecore.ResourceQuery) (resourcecore.ResourcePage, error)
	describe  func(context.Context, resourcecore.ResourceKey) (string, error)
	effective func(context.Context, resourcecore.ResourceScope) ([]resourcecore.EffectiveAttribute, error)
	evaluate  func(context.Context, resourcecore.ResourceScope, []resourcecore.AttributeValue) ([]resourcecore.EffectiveAttribute, error)
	order     func(context.Context, resourcecore.ResourceScope) (resourcecore.ResourceAttributeOrder, error)
}

func (f resourceReaderFuncs) GetResource(ctx context.Context, key resourcecore.ResourceKey) (resourcecore.Resource, error) {
	return f.get(ctx, key)
}

func (f resourceReaderFuncs) SearchResources(ctx context.Context, q resourcecore.ResourceQuery) (resourcecore.ResourcePage, error) {
	return f.search(ctx, q)
}

func (f resourceReaderFuncs) DescribeResource(ctx context.Context, key resourcecore.ResourceKey) (string, error) {
	return f.describe(ctx, key)
}

func (f resourceReaderFuncs) EffectiveAttributesFor(ctx context.Context, scope resourcecore.ResourceScope) ([]resourcecore.EffectiveAttribute, error) {
	return f.effective(ctx, scope)
}

func (f resourceReaderFuncs) EvaluateAttributes(ctx context.Context, scope resourcecore.ResourceScope, values []resourcecore.AttributeValue) ([]resourcecore.EffectiveAttribute, error) {
	return f.evaluate(ctx, scope, values)
}

func (f resourceReaderFuncs) AttributeOrderFor(ctx context.Context, scope resourcecore.ResourceScope) (resourcecore.ResourceAttributeOrder, error) {
	return f.order(ctx, scope)
}

func TestResourceSearchPassesQueryAndMapsPage(t *testing.T) {
	var captured resourcecore.ResourceQuery
	reader := resourceReaderFuncs{search: func(_ context.Context, q resourcecore.ResourceQuery) (resourcecore.ResourcePage, error) {
		captured = q
		return resourcecore.ResourcePage{
			Resources:   []resourcecore.Resource{{ID: 1, IdentityV1: "id-1", Scope: resourcecore.ResourceScope{ClassCode: "C"}}},
			HasPrevious: true, HasNext: true,
		}, nil
	}}
	h := NewRouter(nil, nil, nil, nil, reader, nil, nil, nil)

	for _, tc := range []struct {
		path string
		want resourcecore.ResourceQuery
	}{
		{"/v1/resources", resourcecore.ResourceQuery{Scope: resourcecore.ScopeActive, Limit: 50}},
		{"/v1/resources?scope=ACTIVE&text=x&classCode=C&familyCode=F&typeCode=T&limit=1&offset=2",
			resourcecore.ResourceQuery{Scope: resourcecore.ScopeActive, Text: "x", ClassCode: "C", FamilyCode: "F", TypeCode: "T", Limit: 1, Offset: 2}},
		{"/v1/resources?scope=ALL", resourcecore.ResourceQuery{Scope: resourcecore.ScopeAll, Limit: 50}},
	} {
		t.Run(tc.path, func(t *testing.T) {
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if r.Code != http.StatusOK || !reflect.DeepEqual(captured, tc.want) {
				t.Fatalf("status/query = %d/%#v, want 200/%#v", r.Code, captured, tc.want)
			}
			var page struct {
				Resources   []json.RawMessage `json:"resources"`
				HasPrevious bool              `json:"hasPrevious"`
				HasNext     bool              `json:"hasNext"`
			}
			if err := json.NewDecoder(r.Body).Decode(&page); err != nil {
				t.Fatal(err)
			}
			if len(page.Resources) != 1 || !page.HasPrevious || !page.HasNext {
				t.Fatalf("page = %#v", page)
			}
		})
	}
}

func TestResourceSearchRejectsInvalidParametersWithoutCallingCore(t *testing.T) {
	called := false
	reader := resourceReaderFuncs{search: func(context.Context, resourcecore.ResourceQuery) (resourcecore.ResourcePage, error) {
		called = true
		return resourcecore.ResourcePage{}, nil
	}}
	h := NewRouter(nil, nil, nil, nil, reader, nil, nil, nil)
	for _, query := range []string{"limit=0", "limit=51", "limit=bad", "offset=-1", "offset=bad", "scope=RETIRED"} {
		t.Run(query, func(t *testing.T) {
			called = false
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/resources?"+query, nil))
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

func TestResourceSearchSanitizesNilReader(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/resources", nil))
	var got errorResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if r.Code != http.StatusInternalServerError || got.Error != "internal server error" {
		t.Fatalf("status/error = %d/%q", r.Code, got.Error)
	}
}

func TestResourceDetailMapsRecordAndSanitizesFailures(t *testing.T) {
	cases := []struct {
		name    string
		reader  ResourceReader
		path    string
		status  int
		message string
	}{
		{"happy path", resourceReaderFuncs{get: func(_ context.Context, key resourcecore.ResourceKey) (resourcecore.Resource, error) {
			if key != (resourcecore.ResourceKey{ClassCode: "MATERIAL", IdentityV1: "abc"}) {
				t.Fatalf("key = %#v", key)
			}
			return resourcecore.Resource{ID: 1, IdentityV1: "abc", Scope: resourcecore.ResourceScope{ClassCode: "MATERIAL"}}, nil
		}}, "/v1/resources/MATERIAL/abc", http.StatusOK, ""},
		{"nil reader", nil, "/v1/resources/MATERIAL/abc", http.StatusInternalServerError, "internal server error"},
		{"not found", resourceReaderFuncs{get: func(context.Context, resourcecore.ResourceKey) (resourcecore.Resource, error) {
			return resourcecore.Resource{}, resourcecore.NewError(resourcecore.NotFound, "postgres://user:secret@host/db")
		}}, "/v1/resources/MATERIAL/abc", http.StatusNotFound, "not found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewRouter(nil, nil, nil, nil, tc.reader, nil, nil, nil)
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

func TestResourceDetailRejectsWrongMethod(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, resourceReaderFuncs{}, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/resources/MATERIAL/abc", nil))
	if r.Code != http.StatusMethodNotAllowed || r.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("status/allow = %d/%q", r.Code, r.Header().Get("Allow"))
	}
}
