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

func TestServeTypeAttributeOrderGetPassesScopeAndMapsResponse(t *testing.T) {
	var captured resourcecore.ResourceScope
	reader := resourceReaderFuncs{order: func(_ context.Context, scope resourcecore.ResourceScope) (resourcecore.ResourceAttributeOrder, error) {
		captured = scope
		return resourcecore.ResourceAttributeOrder{
			Scope: scope,
			OrderedAttributes: []resourcecore.AttributeOrderKey{
				{SourceLevel: "TYPE", SourceCode: "CABLE", CharacteristicCode: "color"},
				{SourceLevel: "FAMILY", SourceCode: "CONDUCTORES", CharacteristicCode: "insulation"},
			},
			OrderRevision: "v1:abc123",
		}, nil
	}}
	h := NewRouter(nil, nil, nil, nil, reader, nil, nil, nil)

	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/types/CABLE/attributes/order?classCode=MATERIAL&familyCode=CONDUCTORES", nil))

	if r.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", r.Code, r.Body.String())
	}
	want := resourcecore.ResourceScope{ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE"}
	if captured != want {
		t.Fatalf("scope = %+v, want %+v", captured, want)
	}

	var response attributeOrderResponse
	if err := json.NewDecoder(r.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	wantResponse := attributeOrderResponse{
		Scope: resourceScopeResponse{ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE"},
		OrderedAttributes: []attributeOrderKeyResponse{
			{SourceLevel: "TYPE", SourceCode: "CABLE", CharacteristicCode: "color"},
			{SourceLevel: "FAMILY", SourceCode: "CONDUCTORES", CharacteristicCode: "insulation"},
		},
		OrderRevision: "v1:abc123",
	}
	if response.Scope != wantResponse.Scope || response.OrderRevision != wantResponse.OrderRevision ||
		len(response.OrderedAttributes) != len(wantResponse.OrderedAttributes) {
		t.Fatalf("response = %+v, want %+v", response, wantResponse)
	}
	for i := range wantResponse.OrderedAttributes {
		if response.OrderedAttributes[i] != wantResponse.OrderedAttributes[i] {
			t.Fatalf("ordered attribute[%d] = %+v, want %+v", i, response.OrderedAttributes[i], wantResponse.OrderedAttributes[i])
		}
	}
}

func TestServeTypeAttributeOrderGetPropagatesReaderError(t *testing.T) {
	reader := resourceReaderFuncs{order: func(context.Context, resourcecore.ResourceScope) (resourcecore.ResourceAttributeOrder, error) {
		return resourcecore.ResourceAttributeOrder{}, resourcecore.NewError(resourcecore.InvalidArgument, "family code is required")
	}}
	h := NewRouter(nil, nil, nil, nil, reader, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/types/CABLE/attributes/order?classCode=MATERIAL", nil))
	if r.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", r.Code, r.Body.String())
	}
}

func TestServeTypeAttributeOrderGetNotFound(t *testing.T) {
	reader := resourceReaderFuncs{order: func(context.Context, resourcecore.ResourceScope) (resourcecore.ResourceAttributeOrder, error) {
		return resourcecore.ResourceAttributeOrder{}, resourcecore.NewError(resourcecore.NotFound, "type not found")
	}}
	h := NewRouter(nil, nil, nil, nil, reader, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/types/CABLE/attributes/order?classCode=MATERIAL&familyCode=CONDUCTORES", nil))
	if r.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", r.Code, r.Body.String())
	}
}

func TestServeTypeAttributeOrderGetNilReader(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/types/CABLE/attributes/order?classCode=MATERIAL&familyCode=CONDUCTORES", nil))
	if r.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body = %s", r.Code, r.Body.String())
	}
}

func TestServeTypeAttributeOrderPutMapsRequestAndResponse(t *testing.T) {
	var captured resourcecore.AttributeOrderWriteRequest
	writer := resourceWriterFuncs{order: func(_ context.Context, req resourcecore.AttributeOrderWriteRequest) (resourcecore.ResourceAttributeOrder, error) {
		captured = req
		return resourcecore.ResourceAttributeOrder{
			Scope:             req.Scope,
			OrderedAttributes: req.OrderedAttributes,
			OrderRevision:     "v1:def456",
		}, nil
	}}
	h := NewRouter(nil, nil, writer, nil, nil, nil, nil, nil)
	body := `{
		"actor": "tester",
		"expectedOrderRevision": "v1:abc123",
		"orderedAttributes": [
			{"sourceLevel": "FAMILY", "sourceCode": "CONDUCTORES", "characteristicCode": "insulation"},
			{"sourceLevel": "TYPE", "sourceCode": "CABLE", "characteristicCode": "color"}
		]
	}`
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPut, "/v1/types/CABLE/attributes/order?classCode=MATERIAL&familyCode=CONDUCTORES", strings.NewReader(body)))

	wantRequest := resourcecore.AttributeOrderWriteRequest{
		Actor:                 "tester",
		Scope:                 resourcecore.ResourceScope{ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE"},
		ExpectedOrderRevision: "v1:abc123",
		OrderedAttributes: []resourcecore.AttributeOrderKey{
			{SourceLevel: "FAMILY", SourceCode: "CONDUCTORES", CharacteristicCode: "insulation"},
			{SourceLevel: "TYPE", SourceCode: "CABLE", CharacteristicCode: "color"},
		},
	}
	if captured.Actor != wantRequest.Actor || captured.Scope != wantRequest.Scope ||
		captured.ExpectedOrderRevision != wantRequest.ExpectedOrderRevision ||
		len(captured.OrderedAttributes) != len(wantRequest.OrderedAttributes) {
		t.Fatalf("request = %#v, want %#v", captured, wantRequest)
	}
	for i := range wantRequest.OrderedAttributes {
		if captured.OrderedAttributes[i] != wantRequest.OrderedAttributes[i] {
			t.Fatalf("ordered attribute[%d] = %+v, want %+v", i, captured.OrderedAttributes[i], wantRequest.OrderedAttributes[i])
		}
	}
	if r.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", r.Code, r.Body.String())
	}
	var response attributeOrderResponse
	if err := json.NewDecoder(r.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.OrderRevision != "v1:def456" {
		t.Fatalf("orderRevision = %q, want v1:def456", response.OrderRevision)
	}
}

func TestServeTypeAttributeOrderPutRejectsInvalidBodyWithoutCallingCore(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"invalid JSON", "{not json"},
		{"missing actor", `{"expectedOrderRevision":"v1:a","orderedAttributes":[{"sourceLevel":"TYPE","sourceCode":"CABLE","characteristicCode":"color"}]}`},
		{"blank actor", `{"actor":"","expectedOrderRevision":"v1:a","orderedAttributes":[{"sourceLevel":"TYPE","sourceCode":"CABLE","characteristicCode":"color"}]}`},
		{"missing expectedOrderRevision", `{"actor":"a","orderedAttributes":[{"sourceLevel":"TYPE","sourceCode":"CABLE","characteristicCode":"color"}]}`},
		{"blank expectedOrderRevision", `{"actor":"a","expectedOrderRevision":"","orderedAttributes":[{"sourceLevel":"TYPE","sourceCode":"CABLE","characteristicCode":"color"}]}`},
		{"empty orderedAttributes", `{"actor":"a","expectedOrderRevision":"v1:a","orderedAttributes":[]}`},
		{"missing orderedAttributes", `{"actor":"a","expectedOrderRevision":"v1:a"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			writer := resourceWriterFuncs{order: func(context.Context, resourcecore.AttributeOrderWriteRequest) (resourcecore.ResourceAttributeOrder, error) {
				called = true
				return resourcecore.ResourceAttributeOrder{}, nil
			}}
			h := NewRouter(nil, nil, writer, nil, nil, nil, nil, nil)
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodPut, "/v1/types/CABLE/attributes/order?classCode=MATERIAL&familyCode=CONDUCTORES", strings.NewReader(tc.body)))
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

func TestServeTypeAttributeOrderPutNilWriter(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, nil)
	r := httptest.NewRecorder()
	body := `{"actor":"a","expectedOrderRevision":"v1:a","orderedAttributes":[{"sourceLevel":"TYPE","sourceCode":"CABLE","characteristicCode":"color"}]}`
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPut, "/v1/types/CABLE/attributes/order?classCode=MATERIAL&familyCode=CONDUCTORES", strings.NewReader(body)))
	if r.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body = %s", r.Code, r.Body.String())
	}
}

func TestServeTypeAttributeOrderPutMapsAndSanitizesCoreErrors(t *testing.T) {
	for _, tc := range []struct {
		code    resourcecore.ErrorCode
		status  int
		message string
	}{
		{resourcecore.Conflict, http.StatusConflict, "conflict"},
		{resourcecore.Validation, http.StatusUnprocessableEntity, "validation failed"},
		{resourcecore.Unavailable, http.StatusServiceUnavailable, "unavailable"},
	} {
		t.Run(string(tc.code), func(t *testing.T) {
			writer := resourceWriterFuncs{order: func(context.Context, resourcecore.AttributeOrderWriteRequest) (resourcecore.ResourceAttributeOrder, error) {
				return resourcecore.ResourceAttributeOrder{}, resourcecore.NewError(tc.code, "postgres://user:secret@host/db")
			}}
			h := NewRouter(nil, nil, writer, nil, nil, nil, nil, nil)
			r := httptest.NewRecorder()
			body := `{"actor":"a","expectedOrderRevision":"v1:a","orderedAttributes":[{"sourceLevel":"TYPE","sourceCode":"CABLE","characteristicCode":"color"}]}`
			h.ServeHTTP(r, httptest.NewRequest(http.MethodPut, "/v1/types/CABLE/attributes/order?classCode=MATERIAL&familyCode=CONDUCTORES", strings.NewReader(body)))
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

func TestTypeAttributeOrderRejectsUnsupportedMethod(t *testing.T) {
	h := NewRouter(nil, nil, resourceWriterFuncs{}, nil, resourceReaderFuncs{}, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/types/CABLE/attributes/order?classCode=MATERIAL&familyCode=CONDUCTORES", nil))
	if r.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", r.Code)
	}
	allow := r.Header().Get("Allow")
	if allow != http.MethodGet+", "+http.MethodPut {
		t.Fatalf("Allow = %q", allow)
	}
}

func TestAttributeOrderWriteRequestOpenAPIShape(t *testing.T) {
	schema := catalogSchema(t, "AttributeOrderWriteRequest")
	valid := map[string]any{
		"actor": "a", "expectedOrderRevision": "v1:a",
		"orderedAttributes": []any{
			map[string]any{"sourceLevel": "TYPE", "sourceCode": "CABLE", "characteristicCode": "color"},
		},
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
		t.Fatal("extra property matched AttributeOrderWriteRequest")
	}
	for _, key := range []string{"actor", "expectedOrderRevision", "orderedAttributes"} {
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
