package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/GARFEX33/garfex-costos-unitarios/resourcecore"
)

// characteristicResponse is the public representation of one Core
// CharacteristicDescriptor.
type characteristicResponse struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	ValueType string `json:"valueType"`
	Dimension string `json:"dimension,omitempty"`
}

type effectiveAttributeSourceResponse struct {
	Level string `json:"level"`
	Code  string `json:"code"`
}

type effectiveAttributeOptionResponse struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

// effectiveAttributeResponse is the public representation of one resolved
// attribute: inheritance, presentation order, and rule evaluation are
// already applied by Core — this is never re-derived client-side.
type effectiveAttributeResponse struct {
	Characteristic       characteristicResponse             `json:"characteristic"`
	EffectiveMode        string                             `json:"effectiveMode"`
	IdentityParticipates bool                               `json:"identityParticipates"`
	NotApplicable        bool                               `json:"notApplicable"`
	Position             int                                `json:"position"`
	HasPosition          bool                               `json:"hasPosition"`
	OptionSetCode        string                             `json:"optionSetCode,omitempty"`
	Source               effectiveAttributeSourceResponse   `json:"source"`
	Rules                []applicabilityRuleResponse        `json:"rules"`
	Options              []effectiveAttributeOptionResponse `json:"options"`
}

type effectiveAttributesResponse struct {
	TypeCode   string                       `json:"typeCode"`
	Attributes []effectiveAttributeResponse `json:"attributes"`
}

func serveEffectiveAttributes(w http.ResponseWriter, r *http.Request, reader ResourceReader, typeCode string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	if reader == nil {
		writeCatalogError(w, resourcecore.NewError(resourcecore.Internal, "resource reader unavailable"))
		return
	}
	scope := resourcecore.ResourceScope{
		ClassCode: r.URL.Query().Get("classCode"), FamilyCode: r.URL.Query().Get("familyCode"), TypeCode: typeCode,
	}
	attributes, err := reader.EffectiveAttributesFor(r.Context(), scope)
	if err != nil {
		writeCatalogError(w, err)
		return
	}
	response, err := mapEffectiveAttributesResponse(typeCode, attributes)
	if err != nil {
		writeCatalogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func mapEffectiveAttributesResponse(typeCode string, attributes []resourcecore.EffectiveAttribute) (effectiveAttributesResponse, error) {
	out := make([]effectiveAttributeResponse, len(attributes))
	for i, a := range attributes {
		rules := make([]applicabilityRuleResponse, len(a.Rules))
		for j, rule := range a.Rules {
			equals, err := mapCatalogValue(rule.Equals)
			if err != nil {
				return effectiveAttributesResponse{}, fmt.Errorf("map rule %d: %w", j, err)
			}
			rules[j] = applicabilityRuleResponse{
				AttributeCode: rule.AttributeCode, Equals: equals, Mode: rule.Mode,
				IdentityParticipates: rule.IdentityParticipates, NotApplicable: rule.NotApplicable, Active: rule.Active,
			}
		}
		options := make([]effectiveAttributeOptionResponse, len(a.Options))
		for j, option := range a.Options {
			options[j] = effectiveAttributeOptionResponse{Code: option.Code, Label: option.Label}
		}
		out[i] = effectiveAttributeResponse{
			Characteristic: characteristicResponse{
				Code: a.Characteristic.Code, Name: a.Characteristic.Name,
				ValueType: a.Characteristic.ValueType, Dimension: a.Characteristic.Dimension,
			},
			EffectiveMode: a.EffectiveMode, IdentityParticipates: a.IdentityParticipates, NotApplicable: a.NotApplicable,
			Position: a.Position, HasPosition: a.HasPosition, OptionSetCode: a.OptionSetCode,
			Source:  effectiveAttributeSourceResponse{Level: a.Source.Level, Code: a.Source.Code},
			Rules:   rules,
			Options: options,
		}
	}
	return effectiveAttributesResponse{TypeCode: typeCode, Attributes: out}, nil
}

// evaluateAttributesRequest reuses resourceAttributeRequest's exact code/value
// shape from the resource create/update endpoints, so a frontend already
// building that shape for the Creador does not need a second value encoding.
type evaluateAttributesRequest struct {
	Values []resourceAttributeRequest `json:"values"`
}

func serveEvaluateAttributes(w http.ResponseWriter, r *http.Request, reader ResourceReader, typeCode string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	if reader == nil {
		writeCatalogError(w, resourcecore.NewError(resourcecore.Internal, "resource reader unavailable"))
		return
	}
	var body evaluateAttributesRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeCatalogError(w, resourcecore.NewError(resourcecore.InvalidArgument, "invalid request body"))
		return
	}
	values := make([]resourcecore.AttributeValue, len(body.Values))
	for i, attr := range body.Values {
		value, err := parseCatalogValue(attr.Value)
		if err != nil {
			writeCatalogError(w, err)
			return
		}
		values[i] = resourcecore.AttributeValue{Code: attr.Code, Value: value}
	}
	scope := resourcecore.ResourceScope{
		ClassCode: r.URL.Query().Get("classCode"), FamilyCode: r.URL.Query().Get("familyCode"), TypeCode: typeCode,
	}
	attributes, err := reader.EvaluateAttributes(r.Context(), scope, values)
	if err != nil {
		writeCatalogError(w, err)
		return
	}
	response, err := mapEffectiveAttributesResponse(typeCode, attributes)
	if err != nil {
		writeCatalogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}
