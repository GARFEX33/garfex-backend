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

func TestServeEffectiveAttributesPassesScopeAndMapsResponse(t *testing.T) {
	var captured resourcecore.ResourceScope
	reader := resourceReaderFuncs{effective: func(_ context.Context, scope resourcecore.ResourceScope) ([]resourcecore.EffectiveAttribute, error) {
		captured = scope
		return []resourcecore.EffectiveAttribute{{
			Characteristic:       resourcecore.CharacteristicDescriptor{Code: "color", Name: "Color", ValueType: "CONTROLLED_OPTION"},
			EffectiveMode:        "CONDITIONAL",
			IdentityParticipates: true,
			Position:             3,
			HasPosition:          true,
			OptionSetCode:        "DEFAULT",
			Source:               resourcecore.EffectiveAttributeSource{Level: "TYPE", Code: "CABLE"},
			Rules: []resourcecore.ApplicabilityRule{{
				AttributeCode: "insulation", Equals: resourcecore.Value{Kind: resourcecore.ValueText, Text: "DESNUDO"},
				Mode: "FORBIDDEN", NotApplicable: true, Active: true,
			}},
		}}, nil
	}}
	h := NewRouter(nil, nil, nil, nil, reader, nil, nil, nil)

	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/types/CABLE/attributes/effective?classCode=MATERIAL&familyCode=CONDUCTORES", nil))

	if r.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", r.Code, r.Body.String())
	}
	want := resourcecore.ResourceScope{ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE"}
	if captured != want {
		t.Fatalf("scope = %+v, want %+v", captured, want)
	}

	// applicabilityRuleResponse.Equals is a closed interface (catalogValueResponse)
	// with no UnmarshalJSON — Marshal-only, like every other response DTO that
	// embeds it. Decode into a generic shape instead of the typed response.
	var response map[string]any
	if err := json.NewDecoder(r.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	attributes, _ := response["attributes"].([]any)
	if response["typeCode"] != "CABLE" || len(attributes) != 1 {
		t.Fatalf("unexpected response: %+v", response)
	}
	attr, _ := attributes[0].(map[string]any)
	characteristic, _ := attr["characteristic"].(map[string]any)
	if characteristic["code"] != "color" || characteristic["valueType"] != "CONTROLLED_OPTION" {
		t.Fatalf("unexpected characteristic: %+v", characteristic)
	}
	if attr["effectiveMode"] != "CONDITIONAL" || attr["identityParticipates"] != true {
		t.Fatalf("unexpected mode/identity: %+v", attr)
	}
	if attr["position"] != float64(3) || attr["hasPosition"] != true {
		t.Fatalf("unexpected position: %+v", attr)
	}
	source, _ := attr["source"].(map[string]any)
	if source["level"] != "TYPE" || source["code"] != "CABLE" {
		t.Fatalf("unexpected source: %+v", source)
	}
	rules, _ := attr["rules"].([]any)
	if len(rules) != 1 {
		t.Fatalf("unexpected rules: %+v", rules)
	}
	rule, _ := rules[0].(map[string]any)
	if rule["attributeCode"] != "insulation" || rule["mode"] != "FORBIDDEN" {
		t.Fatalf("unexpected rule: %+v", rule)
	}
}

func TestServeEffectiveAttributesMethodNotAllowed(t *testing.T) {
	reader := resourceReaderFuncs{}
	h := NewRouter(nil, nil, nil, nil, reader, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/types/CABLE/attributes/effective", nil))
	if r.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", r.Code)
	}
}

func TestServeEffectiveAttributesPropagatesReaderError(t *testing.T) {
	reader := resourceReaderFuncs{effective: func(context.Context, resourcecore.ResourceScope) ([]resourcecore.EffectiveAttribute, error) {
		return nil, resourcecore.NewError(resourcecore.InvalidArgument, "family code is required")
	}}
	h := NewRouter(nil, nil, nil, nil, reader, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/types/CABLE/attributes/effective?classCode=MATERIAL", nil))
	if r.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", r.Code, r.Body.String())
	}
}

func TestServeEvaluateAttributesParsesBodyForwardsValuesAndMapsOptions(t *testing.T) {
	var capturedScope resourcecore.ResourceScope
	var capturedValues []resourcecore.AttributeValue
	reader := resourceReaderFuncs{evaluate: func(_ context.Context, scope resourcecore.ResourceScope, values []resourcecore.AttributeValue) ([]resourcecore.EffectiveAttribute, error) {
		capturedScope, capturedValues = scope, values
		return []resourcecore.EffectiveAttribute{{
			Characteristic: resourcecore.CharacteristicDescriptor{Code: "diameter_mm", ValueType: "CONTROLLED_OPTION"},
			EffectiveMode:  "REQUIRED",
			Options:        []resourcecore.EffectiveAttributeOption{{Code: "13 mm", Label: "13 mm"}},
		}}, nil
	}}
	h := NewRouter(nil, nil, nil, nil, reader, nil, nil, nil)

	body := strings.NewReader(`{"values":[{"code":"diameter_inch","value":{"kind":"CONTROLLED_OPTION","value":"1/2\""}}]}`)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/types/TUBERIA/attributes/evaluate?classCode=MATERIAL&familyCode=CANALIZACIONES", body))

	if r.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", r.Code, r.Body.String())
	}
	wantScope := resourcecore.ResourceScope{ClassCode: "MATERIAL", FamilyCode: "CANALIZACIONES", TypeCode: "TUBERIA"}
	if capturedScope != wantScope {
		t.Fatalf("scope = %+v, want %+v", capturedScope, wantScope)
	}
	if len(capturedValues) != 1 || capturedValues[0].Code != "diameter_inch" || capturedValues[0].Value.Text != `1/2"` {
		t.Fatalf("unexpected forwarded values: %+v", capturedValues)
	}

	var response map[string]any
	if err := json.NewDecoder(r.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	attributes, _ := response["attributes"].([]any)
	attr, _ := attributes[0].(map[string]any)
	options, _ := attr["options"].([]any)
	option, _ := options[0].(map[string]any)
	if len(options) != 1 || option["code"] != "13 mm" {
		t.Fatalf("unexpected options: %+v", options)
	}
}

func TestServeEvaluateAttributesMethodNotAllowed(t *testing.T) {
	reader := resourceReaderFuncs{}
	h := NewRouter(nil, nil, nil, nil, reader, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/v1/types/CABLE/attributes/evaluate", nil))
	if r.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", r.Code)
	}
}

func TestServeEvaluateAttributesInvalidBody(t *testing.T) {
	reader := resourceReaderFuncs{}
	h := NewRouter(nil, nil, nil, nil, reader, nil, nil, nil)
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/v1/types/CABLE/attributes/evaluate", strings.NewReader("not json")))
	if r.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", r.Code, r.Body.String())
	}
}

func TestServeEvaluateAttributesPropagatesReaderError(t *testing.T) {
	reader := resourceReaderFuncs{evaluate: func(context.Context, resourcecore.ResourceScope, []resourcecore.AttributeValue) ([]resourcecore.EffectiveAttribute, error) {
		return nil, resourcecore.NewError(resourcecore.InvalidArgument, "family code is required")
	}}
	h := NewRouter(nil, nil, nil, nil, reader, nil, nil, nil)
	r := httptest.NewRecorder()
	body := strings.NewReader(`{"values":[]}`)
	r2 := httptest.NewRequest(http.MethodPost, "/v1/types/CABLE/attributes/evaluate?classCode=MATERIAL", body)
	h.ServeHTTP(r, r2)
	if r.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", r.Code, r.Body.String())
	}
}
