package httpapi

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"net/http"
	"net/http/httptest"

	"github.com/GARFEX33/garfex-costos-unitarios/resourcecore"
	"github.com/getkin/kin-openapi/openapi3"
)

type listCatalogReader struct {
	queries []resourcecore.CatalogQuery
	keys    []resourcecore.CatalogKey
	list    func(context.Context, resourcecore.CatalogQuery) (resourcecore.CatalogPage, error)
	get     func(context.Context, resourcecore.CatalogKey) (resourcecore.CatalogRecord, error)
}

func (r *listCatalogReader) CatalogDescriptors(context.Context) ([]resourcecore.CatalogDescriptor, error) {
	return nil, nil
}

func (r *listCatalogReader) ListCatalog(ctx context.Context, q resourcecore.CatalogQuery) (resourcecore.CatalogPage, error) {
	r.queries = append(r.queries, q)
	return r.list(ctx, q)
}

func (r *listCatalogReader) GetCatalog(ctx context.Context, key resourcecore.CatalogKey) (resourcecore.CatalogRecord, error) {
	r.keys = append(r.keys, key)
	return r.get(ctx, key)
}

func TestCatalogListPassesPublicQueryToCore(t *testing.T) {
	reader := &listCatalogReader{list: func(context.Context, resourcecore.CatalogQuery) (resourcecore.CatalogPage, error) {
		return resourcecore.CatalogPage{}, nil
	}}
	h := NewRouter(reader, nil, nil, nil, nil, nil)

	for _, tc := range []struct {
		path string
		want resourcecore.CatalogQuery
	}{
		{"/v1/catalog/UNIDAD", resourcecore.CatalogQuery{Kind: "UNIDAD", Scope: resourcecore.ScopeActive, Limit: 50}},
		{"/v1/catalog/UNIDAD?scope=ACTIVE&text=steel+bar&limit=1&offset=0", resourcecore.CatalogQuery{Kind: "UNIDAD", Scope: resourcecore.ScopeActive, Text: "steel bar", Limit: 1}},
		{"/v1/catalog/UNIDAD?scope=INACTIVE&text=%26&limit=50&offset=5", resourcecore.CatalogQuery{Kind: "UNIDAD", Scope: resourcecore.ScopeInactive, Text: "&", Limit: 50, Offset: 5}},
		{"/v1/catalog/UNIDAD?scope=ALL&limit=49&offset=1", resourcecore.CatalogQuery{Kind: "UNIDAD", Scope: resourcecore.ScopeAll, Limit: 49, Offset: 1}},
	} {
		t.Run(tc.path, func(t *testing.T) {
			reader.queries = nil
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if r.Code != http.StatusOK || !reflect.DeepEqual(reader.queries, []resourcecore.CatalogQuery{tc.want}) {
				t.Fatalf("status/queries = %d/%#v, want 200/%#v", r.Code, reader.queries, tc.want)
			}
			var page catalogPageResponse
			if err := json.NewDecoder(r.Body).Decode(&page); err != nil || page.Records == nil {
				t.Fatalf("response = %#v, error = %v", page, err)
			}
		})
	}
}

func TestCatalogListMapsNonemptyPageThroughHTTP(t *testing.T) {
	reader := &listCatalogReader{list: func(context.Context, resourcecore.CatalogQuery) (resourcecore.CatalogPage, error) {
		return resourcecore.CatalogPage{
			Records: []resourcecore.CatalogRecord{{
				Kind: "UNIDAD", ID: 9007199254740993, Revision: ^uint64(0), Active: false,
				Values: map[string]resourcecore.Value{
					"boolean":   {Kind: resourcecore.ValueBool, Bool: false},
					"decimal":   {Kind: resourcecore.ValueDecimal, Text: "1.20"},
					"integer":   {Kind: resourcecore.ValueInteger, Text: "9007199254740993"},
					"quantity":  {Kind: resourcecore.ValueQuantity, Text: "1.20", UnitCode: "M"},
					"reference": {Kind: resourcecore.ValueReference, Reference: &resourcecore.Reference{Kind: "MATERIAL", ID: 9007199254740993, Code: "MAT"}},
				},
			}},
			HasPrevious: true, HasNext: true,
		}, nil
	}}
	r := httptest.NewRecorder()
	NewRouter(reader, nil, nil, nil, nil, nil).ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/catalog/UNIDAD", nil))
	body := r.Body.Bytes()
	var page struct {
		Records []struct {
			ID       string                     `json:"id"`
			Revision string                     `json:"revision"`
			Values   map[string]json.RawMessage `json:"values"`
		} `json:"records"`
		HasPrevious bool `json:"hasPrevious"`
		HasNext     bool `json:"hasNext"`
	}
	if err := json.Unmarshal(body, &page); err != nil || r.Code != http.StatusOK || len(page.Records) != 1 || !page.HasPrevious || !page.HasNext {
		t.Fatalf("status/page = %d/%#v, decode error = %v", r.Code, page, err)
	}
	if page.Records[0].ID != "9007199254740993" || page.Records[0].Revision != "18446744073709551615" {
		t.Fatalf("numeric strings = %q/%q", page.Records[0].ID, page.Records[0].Revision)
	}
	for field, want := range map[string]string{
		"boolean":   `{"kind":"BOOLEAN","value":false}`,
		"decimal":   `{"kind":"DECIMAL","value":"1.2"}`,
		"integer":   `{"kind":"INTEGER","value":"9007199254740993"}`,
		"quantity":  `{"kind":"QUANTITY","value":"1.2","unitCode":"M"}`,
		"reference": `{"kind":"REFERENCE","reference":{"kind":"MATERIAL","id":"9007199254740993","code":"MAT"}}`,
	} {
		if got := string(page.Records[0].Values[field]); got != want {
			t.Errorf("%s = %s, want %s", field, got, want)
		}
	}
	var input any
	if err := json.Unmarshal(body, &input); err != nil {
		t.Fatal(err)
	}
	if err := catalogSchema(t, "CatalogPage").VisitJSON(input); err != nil {
		t.Fatalf("CatalogPage schema validation failed: %v", err)
	}
}

func TestCatalogListRejectsInvalidParametersWithoutCallingCore(t *testing.T) {
	reader := &listCatalogReader{list: func(context.Context, resourcecore.CatalogQuery) (resourcecore.CatalogPage, error) {
		return resourcecore.CatalogPage{}, nil
	}}
	h := NewRouter(reader, nil, nil, nil, nil, nil)
	for _, query := range []string{
		"limit=", "limit=0", "limit=-1", "limit=51", "limit=bad", "limit=999999999999999999999999999999",
		"limit=%ZZ", "limit=1;bad", "offset=", "offset=-1", "offset=bad", "offset=999999999999999999999999999999", "offset=%ZZ", "scope=", "scope=RETIRED",
	} {
		t.Run(query, func(t *testing.T) {
			reader.queries = nil
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/catalog/UNIDAD?"+query, nil))
			var response errorResponse
			if err := json.NewDecoder(r.Body).Decode(&response); err != nil || r.Code != http.StatusBadRequest || response.Error != "invalid request" || len(reader.queries) != 0 {
				t.Fatalf("status/error/calls = %d/%q/%d, decode error = %v", r.Code, response.Error, len(reader.queries), err)
			}
		})
	}
}

func TestCatalogListDefersUnknownKindToCore(t *testing.T) {
	reader := &listCatalogReader{list: func(_ context.Context, q resourcecore.CatalogQuery) (resourcecore.CatalogPage, error) {
		if q.Kind != "FUTURE" {
			t.Fatalf("kind = %q, want FUTURE", q.Kind)
		}
		return resourcecore.CatalogPage{}, resourcecore.NewError(resourcecore.InvalidArgument, "unsupported catalog kind")
	}}
	r := httptest.NewRecorder()
	NewRouter(reader, nil, nil, nil, nil, nil).ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/catalog/FUTURE", nil))
	var response errorResponse
	if err := json.NewDecoder(r.Body).Decode(&response); err != nil || r.Code != http.StatusBadRequest || response.Error != "invalid request" {
		t.Fatalf("status/error = %d/%q, decode error = %v", r.Code, response.Error, err)
	}
}

func TestCatalogListSanitizesUnavailableReaderAndMappingErrors(t *testing.T) {
	cases := []struct {
		name    string
		reader  CatalogReader
		status  int
		message string
	}{
		{"nil reader", nil, 500, "internal server error"},
		{"core error", &listCatalogReader{list: func(context.Context, resourcecore.CatalogQuery) (resourcecore.CatalogPage, error) {
			return resourcecore.CatalogPage{}, resourcecore.NewError(resourcecore.Unavailable, "postgres://user:secret@host/db")
		}}, 503, "unavailable"},
		{"mapping error", &listCatalogReader{list: func(context.Context, resourcecore.CatalogQuery) (resourcecore.CatalogPage, error) {
			return resourcecore.CatalogPage{Records: []resourcecore.CatalogRecord{{Values: map[string]resourcecore.Value{"future": {Kind: "FUTURE"}}}}}, nil
		}}, 500, "internal server error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRecorder()
			NewRouter(tc.reader, nil, nil, nil, nil, nil).ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/catalog/UNIDAD", nil))
			var response errorResponse
			if err := json.NewDecoder(r.Body).Decode(&response); err != nil || r.Code != tc.status || response.Error != tc.message {
				t.Fatalf("status/error = %d/%q, decode error = %v", r.Code, response.Error, err)
			}
		})
	}
}

func TestCatalogListRejectsUnsupportedMethodAndExtraSegment(t *testing.T) {
	reader := &listCatalogReader{list: func(context.Context, resourcecore.CatalogQuery) (resourcecore.CatalogPage, error) {
		t.Fatal("reader called")
		return resourcecore.CatalogPage{}, nil
	}}
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{http.MethodPatch, "/v1/catalog/UNIDAD", `{"error":"method not allowed"}` + "\n", http.StatusMethodNotAllowed},
		// Detail is now supported; this retains the old route-boundary regression for an extra segment.
		{http.MethodGet, "/v1/catalog/UNIDAD/1/extra", "404 not found\n", http.StatusNotFound},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			r := httptest.NewRecorder()
			NewRouter(reader, nil, nil, nil, nil, nil).ServeHTTP(r, httptest.NewRequest(tc.method, tc.path, nil))
			if r.Code != tc.status || r.Body.String() != tc.body {
				t.Fatalf("status/body = %d/%q", r.Code, r.Body.String())
			}
			if tc.status == http.StatusMethodNotAllowed && r.Header().Get("Allow") != "GET, POST" {
				t.Fatalf("Allow = %q", r.Header().Get("Allow"))
			}
		})
	}
}

func TestCatalogDetailPassesExactPositiveKeysToCore(t *testing.T) {
	reader := &listCatalogReader{get: func(_ context.Context, key resourcecore.CatalogKey) (resourcecore.CatalogRecord, error) {
		return resourcecore.CatalogRecord{Kind: key.Kind, ID: key.ID}, nil
	}}
	for _, tc := range []struct {
		id   string
		want int64
	}{
		{"1", 1}, {"9007199254740993", 9007199254740993}, {"9223372036854775807", math.MaxInt64},
	} {
		t.Run(tc.id, func(t *testing.T) {
			reader.keys = nil
			r := httptest.NewRecorder()
			NewRouter(reader, nil, nil, nil, nil, nil).ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/catalog/UNIDAD/"+tc.id, nil))
			want := resourcecore.CatalogKey{Kind: resourcecore.KindUnit, ID: tc.want}
			if r.Code != http.StatusOK || !reflect.DeepEqual(reader.keys, []resourcecore.CatalogKey{want}) {
				t.Fatalf("status/keys = %d/%#v, want 200/%#v", r.Code, reader.keys, want)
			}
		})
	}
}

func TestCatalogDetailRejectsMalformedIDWithoutCallingCore(t *testing.T) {
	reader := &listCatalogReader{get: func(context.Context, resourcecore.CatalogKey) (resourcecore.CatalogRecord, error) {
		t.Fatal("reader called")
		return resourcecore.CatalogRecord{}, nil
	}}
	for _, id := range []string{"0", "00", "01", "-1", "+1", " 1", "1 ", "1.0", "1e1", "9223372036854775808"} {
		t.Run(id, func(t *testing.T) {
			reader.keys = nil
			r := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/v1/catalog/UNIDAD/1", nil)
			req.URL.Path = "/v1/catalog/UNIDAD/" + id
			NewRouter(reader, nil, nil, nil, nil, nil).ServeHTTP(r, req)
			var response errorResponse
			if err := json.NewDecoder(r.Body).Decode(&response); err != nil || r.Code != http.StatusBadRequest || response.Error != "invalid request" || len(reader.keys) != 0 {
				t.Fatalf("status/error/calls = %d/%q/%d, decode error = %v", r.Code, response.Error, len(reader.keys), err)
			}
		})
	}
}

func TestCatalogDetailDefersUnknownKindToCore(t *testing.T) {
	reader := &listCatalogReader{get: func(_ context.Context, key resourcecore.CatalogKey) (resourcecore.CatalogRecord, error) {
		if key != (resourcecore.CatalogKey{Kind: "FUTURE", ID: 1}) {
			t.Fatalf("key = %#v", key)
		}
		return resourcecore.CatalogRecord{}, resourcecore.NewError(resourcecore.InvalidArgument, "unsupported catalog kind")
	}}
	r := httptest.NewRecorder()
	NewRouter(reader, nil, nil, nil, nil, nil).ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/catalog/FUTURE/1", nil))
	var response errorResponse
	if err := json.NewDecoder(r.Body).Decode(&response); err != nil || r.Code != http.StatusBadRequest || response.Error != "invalid request" {
		t.Fatalf("status/error = %d/%q, decode error = %v", r.Code, response.Error, err)
	}
}

func TestCatalogDetailOpenAPIIDParameterMatchesPositiveInt64(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromData(catalogOpenAPI)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	detail := doc.Paths.Find("/v1/catalog/{kind}/{id}")
	if detail == nil || detail.Get == nil {
		t.Fatal("detail path is missing")
	}
	var idSchema *openapi3.Schema
	for _, parameter := range detail.Get.Parameters {
		if parameter.Value.Name == "id" && parameter.Value.In == "path" {
			idSchema = parameter.Value.Schema.Value
		}
	}
	if idSchema == nil {
		t.Fatal("detail id parameter schema is missing")
	}
	for _, id := range []string{"1", "9007199254740993", "9223372036854775807"} {
		if err := idSchema.VisitJSON(id); err != nil {
			t.Fatalf("schema rejected %q: %v", id, err)
		}
	}
	for _, id := range []string{"0", "01", "-1", "+1", "1\n", "\n1"} {
		if err := idSchema.VisitJSON(id); err == nil {
			t.Fatalf("schema accepted invalid id %q", id)
		}
	}
	if !strings.Contains(idSchema.Description, "9223372036854775807") {
		t.Fatalf("id description = %q, want runtime int64 boundary", idSchema.Description)
	}
	if err := idSchema.VisitJSON("9223372036854775808"); err != nil {
		t.Fatalf("lexical schema rejected runtime-validated overflow: %v", err)
	}
}

func TestCatalogDetailMapsRecordAndSanitizesFailures(t *testing.T) {
	record := resourcecore.CatalogRecord{
		Kind: "UNIDAD", ID: 9007199254740993, Revision: math.MaxUint64, Active: false,
		Values: map[string]resourcecore.Value{"boolean": {Kind: resourcecore.ValueBool}, "empty": {Kind: resourcecore.ValueStringList}},
	}
	h := NewRouter(&listCatalogReader{get: func(context.Context, resourcecore.CatalogKey) (resourcecore.CatalogRecord, error) { return record, nil }}, nil, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/catalog/UNIDAD/9007199254740993", nil))
	var got struct {
		ID       string                     `json:"id"`
		Revision string                     `json:"revision"`
		Active   bool                       `json:"active"`
		Values   map[string]json.RawMessage `json:"values"`
	}
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil || r.Code != http.StatusOK || got.ID != "9007199254740993" || got.Revision != "18446744073709551615" || got.Active || string(got.Values["boolean"]) != `{"kind":"BOOLEAN","value":false}` || string(got.Values["empty"]) != `{"kind":"STRING_LIST","values":[]}` {
		t.Fatalf("status/record = %d/%#v, decode error = %v", r.Code, got, err)
	}
	input := map[string]any{"kind": "UNIDAD", "id": got.ID, "revision": got.Revision, "active": got.Active, "values": map[string]any{"boolean": map[string]any{"kind": "BOOLEAN", "value": false}, "empty": map[string]any{"kind": "STRING_LIST", "values": []any{}}}, "rules": []any{}}
	if err := catalogSchema(t, "CatalogRecord").VisitJSON(input); err != nil {
		t.Fatalf("CatalogRecord schema validation failed: %v", err)
	}
	for _, tc := range []struct {
		name    string
		reader  CatalogReader
		status  int
		message string
	}{
		{"nil reader", nil, 500, "internal server error"},
		{"core error", &listCatalogReader{get: func(context.Context, resourcecore.CatalogKey) (resourcecore.CatalogRecord, error) {
			return resourcecore.CatalogRecord{}, resourcecore.NewError(resourcecore.Unavailable, "postgres://user:secret@host/db")
		}}, 503, "unavailable"},
		{"not found", &listCatalogReader{get: func(context.Context, resourcecore.CatalogKey) (resourcecore.CatalogRecord, error) {
			return resourcecore.CatalogRecord{}, resourcecore.NewError(resourcecore.NotFound, "record 1")
		}}, 404, "not found"},
		{"mapping error", &listCatalogReader{get: func(context.Context, resourcecore.CatalogKey) (resourcecore.CatalogRecord, error) {
			return resourcecore.CatalogRecord{Values: map[string]resourcecore.Value{"future": {Kind: "FUTURE"}}}, nil
		}}, 500, "internal server error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRecorder()
			NewRouter(tc.reader, nil, nil, nil, nil, nil).ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/catalog/UNIDAD/1", nil))
			var response errorResponse
			if err := json.NewDecoder(r.Body).Decode(&response); err != nil || r.Code != tc.status || response.Error != tc.message {
				t.Fatalf("status/error = %d/%q, decode error = %v", r.Code, response.Error, err)
			}
		})
	}
}

func TestCatalogDetailRequiresGETAndRejectsEmptyOrExtraSegments(t *testing.T) {
	reader := &listCatalogReader{get: func(context.Context, resourcecore.CatalogKey) (resourcecore.CatalogRecord, error) {
		t.Fatal("reader called")
		return resourcecore.CatalogRecord{}, nil
	}}
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{http.MethodPost, "/v1/catalog/UNIDAD/1", `{"error":"method not allowed"}` + "\n", http.StatusMethodNotAllowed},
		{http.MethodGet, "/v1/catalog/UNIDAD/", "404 not found\n", http.StatusNotFound},
		{http.MethodGet, "/v1/catalog/UNIDAD/1/extra", "404 not found\n", http.StatusNotFound},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			r := httptest.NewRecorder()
			NewRouter(reader, nil, nil, nil, nil, nil).ServeHTTP(r, httptest.NewRequest(tc.method, tc.path, nil))
			if r.Code != tc.status || r.Body.String() != tc.body {
				t.Fatalf("status/body = %d/%q", r.Code, r.Body.String())
			}
			if tc.status == http.StatusMethodNotAllowed && r.Header().Get("Allow") != http.MethodGet {
				t.Fatalf("Allow = %q", r.Header().Get("Allow"))
			}
		})
	}
}
