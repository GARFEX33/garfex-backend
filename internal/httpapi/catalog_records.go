package httpapi

import (
	"fmt"
	"strconv"

	"github.com/GARFEX33/garfex-costos-unitarios/resourcecore"
)

// catalogRecordResponse is the public representation of a Core catalog record.
type catalogRecordResponse struct {
	Kind     string                          `json:"kind"`
	ID       string                          `json:"id"`
	Revision string                          `json:"revision"`
	Active   bool                            `json:"active"`
	Values   map[string]catalogValueResponse `json:"values"`
	Rules    []applicabilityRuleResponse     `json:"rules"`
}

type catalogPageResponse struct {
	Records     []catalogRecordResponse `json:"records"`
	HasPrevious bool                    `json:"hasPrevious"`
	HasNext     bool                    `json:"hasNext"`
}

// catalogValueResponse is a closed, tagged union. Each implementation has only
// the fields meaningful to its kind; Core's storage-shaped Value is never sent.
type catalogValueResponse interface{ catalogValueResponse() }

type catalogScalarValueResponse struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type catalogBooleanValueResponse struct {
	Kind  string `json:"kind"`
	Value bool   `json:"value"`
}

type catalogQuantityValueResponse struct {
	Kind     string `json:"kind"`
	Value    string `json:"value"`
	UnitCode string `json:"unitCode"`
}

type catalogReferenceValueResponse struct {
	Kind      string            `json:"kind"`
	Reference referenceResponse `json:"reference"`
}

type catalogStringListValueResponse struct {
	Kind   string   `json:"kind"`
	Values []string `json:"values"`
}

type catalogNotApplicableValueResponse struct {
	Kind string `json:"kind"`
}

type referenceResponse struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Code string `json:"code"`
}

type applicabilityRuleResponse struct {
	AttributeCode        string               `json:"attributeCode"`
	Equals               catalogValueResponse `json:"equals"`
	Mode                 string               `json:"mode"`
	IdentityParticipates bool                 `json:"identityParticipates"`
	NotApplicable        bool                 `json:"notApplicable"`
	Active               bool                 `json:"active"`
}

func (catalogScalarValueResponse) catalogValueResponse()        {}
func (catalogBooleanValueResponse) catalogValueResponse()       {}
func (catalogQuantityValueResponse) catalogValueResponse()      {}
func (catalogReferenceValueResponse) catalogValueResponse()     {}
func (catalogStringListValueResponse) catalogValueResponse()    {}
func (catalogNotApplicableValueResponse) catalogValueResponse() {}

func mapCatalogPage(page resourcecore.CatalogPage) (catalogPageResponse, error) {
	records := make([]catalogRecordResponse, len(page.Records))
	for i, record := range page.Records {
		mapped, err := mapCatalogRecord(record)
		if err != nil {
			return catalogPageResponse{}, err
		}
		records[i] = mapped
	}
	return catalogPageResponse{Records: records, HasPrevious: page.HasPrevious, HasNext: page.HasNext}, nil
}

func mapCatalogRecord(record resourcecore.CatalogRecord) (catalogRecordResponse, error) {
	values := make(map[string]catalogValueResponse, len(record.Values))
	for name, value := range record.Values {
		mapped, err := mapCatalogValue(value)
		if err != nil {
			return catalogRecordResponse{}, fmt.Errorf("map value %q: %w", name, err)
		}
		values[name] = mapped
	}
	rules := make([]applicabilityRuleResponse, len(record.Rules))
	for i, rule := range record.Rules {
		equals, err := mapCatalogValue(rule.Equals)
		if err != nil {
			return catalogRecordResponse{}, fmt.Errorf("map rule %d: %w", i, err)
		}
		rules[i] = applicabilityRuleResponse{
			AttributeCode: rule.AttributeCode, Equals: equals, Mode: rule.Mode,
			IdentityParticipates: rule.IdentityParticipates,
			NotApplicable:        rule.NotApplicable, Active: rule.Active,
		}
	}
	return catalogRecordResponse{
		Kind: string(record.Kind), ID: strconv.FormatInt(record.ID, 10),
		Revision: strconv.FormatUint(record.Revision, 10), Active: record.Active,
		Values: values, Rules: rules,
	}, nil
}

func mapCatalogValue(value resourcecore.Value) (catalogValueResponse, error) {
	kind := string(value.Kind)
	switch value.Kind {
	case resourcecore.ValueText, resourcecore.ValueCode, resourcecore.ValueEnum, resourcecore.ValueControlledOption:
		return catalogScalarValueResponse{Kind: kind, Value: value.Text}, nil
	case resourcecore.ValueBool:
		return catalogBooleanValueResponse{Kind: kind, Value: value.Bool}, nil
	case resourcecore.ValueInteger:
		integer, err := strconv.ParseInt(value.Text, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid integer value: %w", err)
		}
		return catalogScalarValueResponse{Kind: kind, Value: strconv.FormatInt(integer, 10)}, nil
	case resourcecore.ValueDecimal, resourcecore.ValueQuantity:
		decimal, err := resourcecore.CanonicalDecimalString(value.Text)
		if err != nil {
			return nil, fmt.Errorf("invalid decimal value: %w", err)
		}
		if value.Kind == resourcecore.ValueDecimal {
			return catalogScalarValueResponse{Kind: kind, Value: decimal}, nil
		}
		return catalogQuantityValueResponse{Kind: kind, Value: decimal, UnitCode: value.UnitCode}, nil
	case resourcecore.ValueReference:
		if value.Reference == nil {
			return nil, fmt.Errorf("reference value has no reference")
		}
		return catalogReferenceValueResponse{
			Kind: kind,
			Reference: referenceResponse{
				Kind: string(value.Reference.Kind), ID: strconv.FormatInt(value.Reference.ID, 10),
				Code: value.Reference.Code,
			},
		}, nil
	case resourcecore.ValueStringList:
		return catalogStringListValueResponse{Kind: kind, Values: stringsOrEmpty(value.Strings)}, nil
	case resourcecore.ValueNotApplicable:
		return catalogNotApplicableValueResponse{Kind: kind}, nil
	default:
		return nil, fmt.Errorf("unknown catalog value kind %q", value.Kind)
	}
}
