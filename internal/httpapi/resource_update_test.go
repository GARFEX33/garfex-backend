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

func TestUpdateResourceMapsRequestAndResponse(t *testing.T) {
	var captured resourcecore.ResourceUpdateRequest
	writer := resourceWriterFuncs{update: func(_ context.Context, req resourcecore.ResourceUpdateRequest) (resourcecore.Resource, error) {
		captured = req
		return resourcecore.Resource{
			ID: req.ID, IdentityV1: "identity-1", Scope: req.Scope,
			NaturalUnit: req.NaturalUnit, Active: true, Revision: req.ExpectedRevision + 1,
			Attributes: req.Attributes,
		}, nil
	}}
	h := NewRouter(nil, nil, writer, nil, nil)
	body := `{
		"actor": "tester",
		"expectedRevision": "3",
		"scope": {"classCode": "C1", "familyCode": "F1", "typeCode": "T1"},
		"naturalUnit": "KG",
		"attributes": [{"code": "peso", "value": {"kind": "QUANTITY", "value": "2.5", "unitCode": "KG"}}]
	}`
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPut, "/v1/resources/42", strings.NewReader(body)))

	wantRequest := resourcecore.ResourceUpdateRequest{
		Actor:            "tester",
		ID:               42,
		ExpectedRevision: 3,
		Scope:            resourcecore.ResourceScope{ClassCode: "C1", FamilyCode: "F1", TypeCode: "T1"},
		NaturalUnit:      "KG",
		Attributes: []resourcecore.AttributeValue{
			{Code: "peso", Value: resourcecore.Value{Kind: resourcecore.ValueQuantity, Text: "2.5", UnitCode: "KG"}},
		},
	}
	if captured.Actor != wantRequest.Actor || captured.ID != wantRequest.ID || captured.ExpectedRevision != wantRequest.ExpectedRevision ||
		captured.Scope != wantRequest.Scope || captured.NaturalUnit != wantRequest.NaturalUnit {
		t.Fatalf("request = %#v, want %#v", captured, wantRequest)
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
}

func TestUpdateResourceRejectsInvalidIDWithoutCallingCore(t *testing.T) {
	called := false
	writer := resourceWriterFuncs{update: func(context.Context, resourcecore.ResourceUpdateRequest) (resourcecore.Resource, error) {
		called = true
		return resourcecore.Resource{}, nil
	}}
	h := NewRouter(nil, nil, writer, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPut, "/v1/resources/0", strings.NewReader(`{"actor":"a","expectedRevision":"1","scope":{"classCode":"C","familyCode":"F","typeCode":"T"},"naturalUnit":"KG"}`)))
	var got errorResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if r.Code != http.StatusBadRequest || got.Error != "invalid request" || called {
		t.Fatalf("status/error/called = %d/%q/%t", r.Code, got.Error, called)
	}
}

func TestUpdateResourceRejectsInvalidBodyWithoutCallingCore(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"invalid JSON", "{not json"},
		{"missing expected revision", `{"actor":"a","scope":{"classCode":"C","familyCode":"F","typeCode":"T"},"naturalUnit":"KG"}`},
		{"non-numeric expected revision", `{"actor":"a","expectedRevision":"bad","scope":{"classCode":"C","familyCode":"F","typeCode":"T"},"naturalUnit":"KG"}`},
		{"unknown value kind", `{"actor":"a","expectedRevision":"1","scope":{"classCode":"C","familyCode":"F","typeCode":"T"},"naturalUnit":"KG","attributes":[{"code":"x","value":{"kind":"FUTURE","value":"x"}}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			writer := resourceWriterFuncs{update: func(context.Context, resourcecore.ResourceUpdateRequest) (resourcecore.Resource, error) {
				called = true
				return resourcecore.Resource{}, nil
			}}
			h := NewRouter(nil, nil, writer, nil, nil)
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodPut, "/v1/resources/1", strings.NewReader(tc.body)))
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

func TestUpdateResourceSanitizesNilWriter(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPut, "/v1/resources/1", strings.NewReader(`{"actor":"a","expectedRevision":"1","scope":{"classCode":"C","familyCode":"F","typeCode":"T"},"naturalUnit":"KG"}`)))
	var got errorResponse
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if r.Code != http.StatusInternalServerError || got.Error != "internal server error" {
		t.Fatalf("status/error = %d/%q", r.Code, got.Error)
	}
}

func TestUpdateResourceMapsAndSanitizesCoreErrors(t *testing.T) {
	for _, tc := range []struct {
		code    resourcecore.ErrorCode
		status  int
		message string
	}{
		{resourcecore.NotFound, 404, "not found"},
		{resourcecore.Conflict, 409, "conflict"},
		{resourcecore.InvalidReference, 422, "validation failed"},
	} {
		t.Run(string(tc.code), func(t *testing.T) {
			writer := resourceWriterFuncs{update: func(context.Context, resourcecore.ResourceUpdateRequest) (resourcecore.Resource, error) {
				return resourcecore.Resource{}, resourcecore.NewError(tc.code, "postgres://user:secret@host/db")
			}}
			h := NewRouter(nil, nil, writer, nil, nil)
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodPut, "/v1/resources/1", strings.NewReader(`{"actor":"a","expectedRevision":"1","scope":{"classCode":"C","familyCode":"F","typeCode":"T"},"naturalUnit":"KG"}`)))
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

func TestResourceUpdateRequestOpenAPIShape(t *testing.T) {
	schema := catalogSchema(t, "ResourceUpdateRequest")
	valid := map[string]any{
		"actor": "a", "expectedRevision": "1",
		"scope":       map[string]any{"classCode": "C", "familyCode": "F", "typeCode": "T"},
		"naturalUnit": "KG", "attributes": []any{},
	}
	if err := schema.VisitJSON(valid); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	withExtra := map[string]any{}
	for k, v := range valid {
		withExtra[k] = v
	}
	withExtra["extra"] = true
	if err := schema.VisitJSON(withExtra); err == nil {
		t.Fatal("extra property matched ResourceUpdateRequest")
	}
	for _, key := range []string{"actor", "expectedRevision", "scope", "naturalUnit", "attributes"} {
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

func TestResourceIDRejectsWrongMethod(t *testing.T) {
	h := NewRouter(nil, nil, resourceWriterFuncs{}, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/resources/1", nil))
	if r.Code != http.StatusMethodNotAllowed || r.Header().Get("Allow") != http.MethodPut {
		t.Fatalf("status/allow = %d/%q", r.Code, r.Header().Get("Allow"))
	}
}
