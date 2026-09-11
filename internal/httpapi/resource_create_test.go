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

type resourceWriterFunc func(context.Context, resourcecore.ResourceWriteRequest) (resourcecore.Resource, error)

func (f resourceWriterFunc) CreateResource(ctx context.Context, req resourcecore.ResourceWriteRequest) (resourcecore.Resource, error) {
	return f(ctx, req)
}

func TestCreateResourceMapsRequestAndResponse(t *testing.T) {
	var captured resourcecore.ResourceWriteRequest
	h := NewRouter(nil, nil, resourceWriterFunc(func(_ context.Context, req resourcecore.ResourceWriteRequest) (resourcecore.Resource, error) {
		captured = req
		return resourcecore.Resource{
			ID: 9007199254740993, IdentityV1: "identity-1",
			Scope: req.Scope, NaturalUnit: req.NaturalUnit, Active: true, Revision: 3,
			Attributes: req.Attributes,
		}, nil
	}))
	body := `{
		"actor": "tester",
		"scope": {"classCode": "C1", "familyCode": "F1", "typeCode": "T1"},
		"naturalUnit": "KG",
		"attributes": [
			{"code": "peso", "value": {"kind": "QUANTITY", "value": "1.20", "unitCode": "KG"}},
			{"code": "activo", "value": {"kind": "BOOLEAN", "value": true}},
			{"code": "nombre", "value": {"kind": "TEXT", "value": "acero"}},
			{"code": "material", "value": {"kind": "REFERENCE", "reference": {"kind": "MATERIAL", "code": "MAT-1"}}},
			{"code": "tags", "value": {"kind": "STRING_LIST", "values": ["a", "b"]}},
			{"code": "na", "value": {"kind": "NOT_APPLICABLE"}}
		]
	}`
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/resources", strings.NewReader(body)))

	wantRequest := resourcecore.ResourceWriteRequest{
		Actor:       "tester",
		Scope:       resourcecore.ResourceScope{ClassCode: "C1", FamilyCode: "F1", TypeCode: "T1"},
		NaturalUnit: "KG",
		Attributes: []resourcecore.AttributeValue{
			{Code: "peso", Value: resourcecore.Value{Kind: resourcecore.ValueQuantity, Text: "1.20", UnitCode: "KG"}},
			{Code: "activo", Value: resourcecore.Value{Kind: resourcecore.ValueBool, Bool: true}},
			{Code: "nombre", Value: resourcecore.Value{Kind: resourcecore.ValueText, Text: "acero"}},
			{Code: "material", Value: resourcecore.Value{Kind: resourcecore.ValueReference, Reference: &resourcecore.Reference{Kind: "MATERIAL", Code: "MAT-1"}}},
			{Code: "tags", Value: resourcecore.Value{Kind: resourcecore.ValueStringList, Strings: []string{"a", "b"}}},
			{Code: "na", Value: resourcecore.Value{Kind: resourcecore.ValueNotApplicable}},
		},
	}
	if !reflect.DeepEqual(captured, wantRequest) {
		t.Fatalf("request = %#v, want %#v", captured, wantRequest)
	}
	if r.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", r.Code, r.Body.String())
	}
	body2 := r.Body.Bytes()
	var input any
	if err := json.Unmarshal(body2, &input); err != nil {
		t.Fatal(err)
	}
	if err := catalogSchema(t, "Resource").VisitJSON(input); err != nil {
		t.Fatalf("Resource schema validation failed: %v", err)
	}
	var response struct {
		ID         string `json:"id"`
		IdentityV1 string `json:"identityV1"`
		Revision   string `json:"revision"`
		Active     bool   `json:"active"`
		Attributes []struct {
			Code  string          `json:"code"`
			Value json.RawMessage `json:"value"`
		} `json:"attributes"`
	}
	if err := json.Unmarshal(body2, &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != "9007199254740993" || response.IdentityV1 != "identity-1" || response.Revision != "3" || !response.Active {
		t.Fatalf("response = %#v", response)
	}
	if len(response.Attributes) != 6 || response.Attributes[3].Code != "material" {
		t.Fatalf("attributes = %#v", response.Attributes)
	}
}

func TestCreateResourceRequiresPOST(t *testing.T) {
	called := false
	h := NewRouter(nil, nil, resourceWriterFunc(func(context.Context, resourcecore.ResourceWriteRequest) (resourcecore.Resource, error) {
		called = true
		return resourcecore.Resource{}, nil
	}))
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/resources", nil))
	var got errorResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if r.Code != http.StatusMethodNotAllowed || r.Header().Get("Allow") != http.MethodPost || got.Error != "method not allowed" || called {
		t.Fatalf("status/allow/error/called = %d/%q/%q/%t", r.Code, r.Header().Get("Allow"), got.Error, called)
	}
}

func TestCreateResourceRejectsInvalidBodyWithoutCallingCore(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"invalid JSON", "{not json"},
		{"unknown value kind", `{"actor":"a","scope":{"classCode":"C","familyCode":"F","typeCode":"T"},"naturalUnit":"KG","attributes":[{"code":"x","value":{"kind":"FUTURE","value":"x"}}]}`},
		{"extra field on TEXT value", `{"actor":"a","scope":{"classCode":"C","familyCode":"F","typeCode":"T"},"naturalUnit":"KG","attributes":[{"code":"x","value":{"kind":"TEXT","value":"x","extra":true}}]}`},
		{"missing quantity unit", `{"actor":"a","scope":{"classCode":"C","familyCode":"F","typeCode":"T"},"naturalUnit":"KG","attributes":[{"code":"x","value":{"kind":"QUANTITY","value":"1"}}]}`},
		{"reference with id", `{"actor":"a","scope":{"classCode":"C","familyCode":"F","typeCode":"T"},"naturalUnit":"KG","attributes":[{"code":"x","value":{"kind":"REFERENCE","reference":{"kind":"MATERIAL","id":"1","code":"M"}}}]}`},
		{"boolean value is a string", `{"actor":"a","scope":{"classCode":"C","familyCode":"F","typeCode":"T"},"naturalUnit":"KG","attributes":[{"code":"x","value":{"kind":"BOOLEAN","value":"true"}}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			h := NewRouter(nil, nil, resourceWriterFunc(func(context.Context, resourcecore.ResourceWriteRequest) (resourcecore.Resource, error) {
				called = true
				return resourcecore.Resource{}, nil
			}))
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/resources", strings.NewReader(tc.body)))
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

func TestCreateResourceSanitizesNilWriter(t *testing.T) {
	h := NewRouter(nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/resources", strings.NewReader(`{"actor":"tester","scope":{"classCode":"C","familyCode":"F","typeCode":"T"},"naturalUnit":"KG"}`)))
	var got errorResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if r.Code != http.StatusInternalServerError || got.Error != "internal server error" {
		t.Fatalf("status/error = %d/%q", r.Code, got.Error)
	}
}

func TestCreateResourceMapsAndSanitizesCoreErrors(t *testing.T) {
	for _, tc := range []struct {
		code    resourcecore.ErrorCode
		status  int
		message string
	}{
		{resourcecore.InvalidArgument, 400, "invalid request"}, {resourcecore.NotFound, 404, "not found"},
		{resourcecore.Duplicate, 409, "conflict"}, {resourcecore.IdentityConflict, 409, "conflict"},
		{resourcecore.InvalidReference, 422, "validation failed"}, {resourcecore.Validation, 422, "validation failed"},
		{resourcecore.Unavailable, 503, "unavailable"}, {resourcecore.Internal, 500, "internal server error"},
	} {
		t.Run(string(tc.code), func(t *testing.T) {
			h := NewRouter(nil, nil, resourceWriterFunc(func(context.Context, resourcecore.ResourceWriteRequest) (resourcecore.Resource, error) {
				return resourcecore.Resource{}, resourcecore.NewError(tc.code, "postgres://user:secret@host/db")
			}))
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/resources", strings.NewReader(`{"actor":"a","scope":{"classCode":"C","familyCode":"F","typeCode":"T"},"naturalUnit":"KG"}`)))
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

func TestResourceOpenAPIRejectsExtraProperties(t *testing.T) {
	if err := catalogSchema(t, "ResourceCreateRequest").VisitJSON(map[string]any{
		"actor": "a", "scope": map[string]any{"classCode": "C", "familyCode": "F", "typeCode": "T"},
		"naturalUnit": "KG", "attributes": []any{}, "extra": true,
	}); err == nil {
		t.Fatal("extra property matched ResourceCreateRequest")
	}
	if err := catalogSchema(t, "Resource").VisitJSON(map[string]any{
		"id": "1", "identityV1": "", "scope": map[string]any{"classCode": "C", "familyCode": "F", "typeCode": "T"},
		"naturalUnit": "KG", "active": true, "revision": "1", "attributes": []any{}, "extra": true,
	}); err == nil {
		t.Fatal("extra property matched Resource")
	}
}

func TestResourceCreateRequestRequiresAllFields(t *testing.T) {
	schema := catalogSchema(t, "ResourceCreateRequest")
	valid := map[string]any{
		"actor": "a", "scope": map[string]any{"classCode": "C", "familyCode": "F", "typeCode": "T"},
		"naturalUnit": "KG", "attributes": []any{},
	}
	if err := schema.VisitJSON(valid); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	for _, key := range []string{"actor", "scope", "naturalUnit", "attributes"} {
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

func TestParseCatalogValueUnknownError(t *testing.T) {
	if _, err := parseCatalogValue(json.RawMessage(`{"kind":"FUTURE"}`)); resourcecore.Code(err) != resourcecore.InvalidArgument {
		t.Fatalf("code = %v, want INVALID_ARGUMENT", resourcecore.Code(err))
	}
	if _, err := parseCatalogValue(json.RawMessage(`not json`)); resourcecore.Code(err) != resourcecore.InvalidArgument {
		t.Fatalf("code = %v, want INVALID_ARGUMENT", resourcecore.Code(err))
	}
	if _, err := parseCatalogValue(json.RawMessage(`{}`)); resourcecore.Code(err) != resourcecore.InvalidArgument {
		t.Fatalf("code = %v, want INVALID_ARGUMENT", resourcecore.Code(err))
	}
	if status, _ := catalogError(errors.New("secret")); status != http.StatusInternalServerError {
		t.Fatalf("unknown error status = %d, want 500", status)
	}
}
