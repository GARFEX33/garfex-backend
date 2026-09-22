package httpapi

import (
	"context"
	_ "embed"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/GARFEX33/garfex-costos-unitarios/resourcecore"
	"github.com/getkin/kin-openapi/openapi3"
)

//go:embed openapi.yaml
var catalogOpenAPI []byte

func TestMapCatalogValueVariants(t *testing.T) {
	schema := catalogSchema(t, "CatalogValue")
	cases := []struct {
		name  string
		value resourcecore.Value
		want  string
	}{
		{"text", resourcecore.Value{Kind: resourcecore.ValueText, Text: ""}, `{"kind":"TEXT","value":""}`},
		{"code", resourcecore.Value{Kind: resourcecore.ValueCode, Text: "C"}, `{"kind":"CODE","value":"C"}`},
		{"bool", resourcecore.Value{Kind: resourcecore.ValueBool, Bool: false}, `{"kind":"BOOLEAN","value":false}`},
		{"integer", resourcecore.Value{Kind: resourcecore.ValueInteger, Text: "9007199254740993"}, `{"kind":"INTEGER","value":"9007199254740993"}`},
		{"decimal", resourcecore.Value{Kind: resourcecore.ValueDecimal, Text: "1.20"}, `{"kind":"DECIMAL","value":"1.2"}`},
		{"quantity", resourcecore.Value{Kind: resourcecore.ValueQuantity, Text: "1.20", UnitCode: ""}, `{"kind":"QUANTITY","value":"1.2","unitCode":""}`},
		{"reference", resourcecore.Value{Kind: resourcecore.ValueReference, Reference: &resourcecore.Reference{Kind: "UNIT", ID: math.MaxInt64, Code: ""}}, `{"kind":"REFERENCE","reference":{"kind":"UNIT","id":"9223372036854775807","code":""}}`},
		{"enum", resourcecore.Value{Kind: resourcecore.ValueEnum, Text: "A"}, `{"kind":"ENUM","value":"A"}`},
		{"controlled option", resourcecore.Value{Kind: resourcecore.ValueControlledOption, Text: "O"}, `{"kind":"CONTROLLED_OPTION","value":"O"}`},
		{"string list", resourcecore.Value{Kind: resourcecore.ValueStringList}, `{"kind":"STRING_LIST","values":[]}`},
		{"not applicable", resourcecore.Value{Kind: resourcecore.ValueNotApplicable}, `{"kind":"NOT_APPLICABLE"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := mapCatalogValue(tc.value)
			if err != nil {
				t.Fatal(err)
			}
			body, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			if string(body) != tc.want {
				t.Fatalf("JSON = %s, want %s", body, tc.want)
			}
			validateSchemaJSON(t, schema, got)
		})
	}
}

func TestMapCatalogValueFailsClosedForUnknownKind(t *testing.T) {
	if _, err := mapCatalogValue(resourcecore.Value{Kind: "FUTURE"}); err == nil {
		t.Fatal("unknown kind mapped successfully")
	}
}

func TestMapCatalogRecordAndPage(t *testing.T) {
	record := resourcecore.CatalogRecord{
		Kind: "APLICABILIDAD", ID: math.MaxInt64, Revision: math.MaxUint64, Active: false,
		Values: map[string]resourcecore.Value{"empty": {Kind: resourcecore.ValueText}},
		Rules:  []resourcecore.ApplicabilityRule{{AttributeCode: "material", Equals: resourcecore.Value{Kind: resourcecore.ValueReference, Reference: &resourcecore.Reference{Kind: "CLASE", ID: math.MaxInt64, Code: ""}}, Mode: "ALL", IdentityParticipates: false, NotApplicable: false, Active: false}},
	}
	got, err := mapCatalogRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "9223372036854775807" || got.Revision != "18446744073709551615" || got.Rules == nil || got.Values["empty"] == nil {
		t.Fatalf("record = %#v", got)
	}
	if body, err := json.Marshal(got); err != nil || strings.Contains(string(body), `"bool"`) || !strings.Contains(string(body), `"active":false`) {
		t.Fatalf("record JSON = %s, err = %v", body, err)
	}
	validateSchemaJSON(t, catalogSchema(t, "CatalogRecord"), got)
	page, err := mapCatalogPage(resourcecore.CatalogPage{Records: []resourcecore.CatalogRecord{record}})
	if err != nil {
		t.Fatal(err)
	}
	if page.Records == nil || len(page.Records) != 1 || page.HasPrevious || page.HasNext {
		t.Fatalf("page = %#v", page)
	}
	validateSchemaJSON(t, catalogSchema(t, "CatalogPage"), page)
}

func TestCatalogMappingPropagatesInvalidValues(t *testing.T) {
	nilReference := resourcecore.Value{Kind: resourcecore.ValueReference}
	unknownKind := resourcecore.Value{Kind: "FUTURE"}
	tests := []struct {
		name  string
		mapFn func() error
	}{
		{
			name: "record value nil reference",
			mapFn: func() error {
				_, err := mapCatalogRecord(resourcecore.CatalogRecord{Values: map[string]resourcecore.Value{"ref": nilReference}})
				return err
			},
		},
		{
			name: "record rule unknown kind",
			mapFn: func() error {
				_, err := mapCatalogRecord(resourcecore.CatalogRecord{Rules: []resourcecore.ApplicabilityRule{{Equals: unknownKind}}})
				return err
			},
		},
		{
			name: "page record value unknown kind",
			mapFn: func() error {
				_, err := mapCatalogPage(resourcecore.CatalogPage{Records: []resourcecore.CatalogRecord{{Values: map[string]resourcecore.Value{"future": unknownKind}}}})
				return err
			},
		},
		{
			name: "page record rule nil reference",
			mapFn: func() error {
				_, err := mapCatalogPage(resourcecore.CatalogPage{Records: []resourcecore.CatalogRecord{{Rules: []resourcecore.ApplicabilityRule{{Equals: nilReference}}}}})
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.mapFn(); err == nil {
				t.Fatal("invalid Core value mapped successfully")
			}
		})
	}
}

func TestCatalogOpenAPIRejectsExtraProperties(t *testing.T) {
	schema := catalogSchema(t, "CatalogValue")
	if err := schema.VisitJSON(map[string]any{"kind": "TEXT", "value": "x", "extra": true}); err == nil {
		t.Fatal("extra property matched CatalogValue")
	}
}

func TestCatalogOpenAPINumericSemantics(t *testing.T) {
	valueSchema := catalogSchema(t, "CatalogValue")
	for _, value := range []resourcecore.Value{
		{Kind: resourcecore.ValueDecimal, Text: "0.1"},
		{Kind: resourcecore.ValueDecimal, Text: "-0.1"},
		{Kind: resourcecore.ValueDecimal, Text: "0"},
		{Kind: resourcecore.ValueQuantity, Text: "0.1"},
	} {
		mapped, err := mapCatalogValue(value)
		if err != nil {
			t.Fatal(err)
		}
		validateSchemaJSON(t, valueSchema, mapped)
	}
	for _, value := range []resourcecore.Value{
		{Kind: resourcecore.ValueInteger, Text: "invalid"},
		{Kind: resourcecore.ValueInteger, Text: "9223372036854775808"},
		{Kind: resourcecore.ValueDecimal, Text: "invalid"},
	} {
		if _, err := mapCatalogValue(value); err == nil {
			t.Fatalf("invalid numeric value %#v mapped successfully", value)
		}
	}
	for _, tc := range []struct {
		name, schema string
		value        any
	}{
		{"integer invalid", "CatalogIntegerValue", map[string]any{"kind": "INTEGER", "value": "01"}},
		{"decimal invalid", "CatalogDecimalValue", map[string]any{"kind": "DECIMAL", "value": "1.0"}},
		{"decimal newline", "CatalogDecimalValue", map[string]any{"kind": "DECIMAL", "value": "0.1\n"}},
		{"quantity newline", "CatalogQuantityValue", map[string]any{"kind": "QUANTITY", "value": "0.1\n", "unitCode": ""}},
		{"record id newline", "CatalogRecord", map[string]any{"kind": "K", "id": "1\n", "revision": "1", "active": true, "values": map[string]any{}, "rules": []any{}}},
		{"record revision newline", "CatalogRecord", map[string]any{"kind": "K", "id": "1", "revision": "1\n", "active": true, "values": map[string]any{}, "rules": []any{}}},
		{"reference id newline", "CatalogReference", map[string]any{"kind": "K", "id": "1\n", "code": ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := catalogSchema(t, tc.schema).VisitJSON(tc.value); err == nil {
				t.Fatal("invalid numeric value matched schema")
			}
		})
	}
}

func TestMapCatalogPageEmptyRecords(t *testing.T) {
	page, err := mapCatalogPage(resourcecore.CatalogPage{})
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"records":[],"hasPrevious":false,"hasNext":false}` {
		t.Fatalf("empty page JSON = %s", body)
	}
	validateSchemaJSON(t, catalogSchema(t, "CatalogPage"), page)
}

func TestMapCatalogNumericCanonicalization(t *testing.T) {
	schema := catalogSchema(t, "CatalogValue")
	for _, tc := range []struct {
		name  string
		value resourcecore.Value
		want  string
	}{
		{"integer minimum", resourcecore.Value{Kind: resourcecore.ValueInteger, Text: "-9223372036854775808"}, `{"kind":"INTEGER","value":"-9223372036854775808"}`},
		{"integer maximum", resourcecore.Value{Kind: resourcecore.ValueInteger, Text: "9223372036854775807"}, `{"kind":"INTEGER","value":"9223372036854775807"}`},
		{"integer negative", resourcecore.Value{Kind: resourcecore.ValueInteger, Text: "-7"}, `{"kind":"INTEGER","value":"-7"}`},
		{"integer zero", resourcecore.Value{Kind: resourcecore.ValueInteger, Text: "0"}, `{"kind":"INTEGER","value":"0"}`},
		{"decimal negative", resourcecore.Value{Kind: resourcecore.ValueDecimal, Text: "-1.20"}, `{"kind":"DECIMAL","value":"-1.2"}`},
		{"decimal zero", resourcecore.Value{Kind: resourcecore.ValueDecimal, Text: "-0.000"}, `{"kind":"DECIMAL","value":"0"}`},
		{"decimal scientific", resourcecore.Value{Kind: resourcecore.ValueDecimal, Text: "1.25e3"}, `{"kind":"DECIMAL","value":"1250"}`},
		{"decimal large", resourcecore.Value{Kind: resourcecore.ValueDecimal, Text: "9223372036854775808.1250"}, `{"kind":"DECIMAL","value":"9223372036854775808.125"}`},
		{"quantity negative", resourcecore.Value{Kind: resourcecore.ValueQuantity, Text: "-1.20", UnitCode: "M"}, `{"kind":"QUANTITY","value":"-1.2","unitCode":"M"}`},
		{"quantity zero", resourcecore.Value{Kind: resourcecore.ValueQuantity, Text: "-0.000", UnitCode: "M"}, `{"kind":"QUANTITY","value":"0","unitCode":"M"}`},
		{"quantity scientific", resourcecore.Value{Kind: resourcecore.ValueQuantity, Text: "1.25e3", UnitCode: "M"}, `{"kind":"QUANTITY","value":"1250","unitCode":"M"}`},
		{"quantity large", resourcecore.Value{Kind: resourcecore.ValueQuantity, Text: "9223372036854775808.1250", UnitCode: "M"}, `{"kind":"QUANTITY","value":"9223372036854775808.125","unitCode":"M"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mapped, err := mapCatalogValue(tc.value)
			if err != nil {
				t.Fatal(err)
			}
			body, err := json.Marshal(mapped)
			if err != nil {
				t.Fatal(err)
			}
			if string(body) != tc.want {
				t.Fatalf("JSON = %s, want %s", body, tc.want)
			}
			validateSchemaJSON(t, schema, mapped)
		})
	}
	for _, value := range []resourcecore.Value{
		{Kind: resourcecore.ValueInteger, Text: "invalid"},
		{Kind: resourcecore.ValueInteger, Text: "9223372036854775808"},
		{Kind: resourcecore.ValueInteger, Text: "-9223372036854775809"},
		{Kind: resourcecore.ValueDecimal, Text: "invalid"},
		{Kind: resourcecore.ValueQuantity, Text: "invalid"},
		{Kind: resourcecore.ValueDecimal, Text: "1e2147483648"},
		{Kind: resourcecore.ValueQuantity, Text: "1e2147483648"},
		{Kind: resourcecore.ValueDecimal, Text: "1e-2147483649"},
		{Kind: resourcecore.ValueQuantity, Text: "1e-2147483649"},
	} {
		if _, err := mapCatalogValue(value); err == nil {
			t.Fatalf("invalid numeric value %#v mapped successfully", value)
		}
	}
}

func TestCatalogOpenAPICanonicalNumericLexemes(t *testing.T) {
	for _, tc := range []struct {
		schema string
		kind   string
		field  string
		values []string
	}{
		{"CatalogIntegerValue", "INTEGER", "value", []string{"0", "-1", "9223372036854775807"}},
		{"CatalogDecimalValue", "DECIMAL", "value", []string{"0", "-0.1", "0.1", "9223372036854775808.125"}},
		{"CatalogQuantityValue", "QUANTITY", "value", []string{"0", "-0.1", "0.1", "9223372036854775808.125"}},
	} {
		for _, value := range tc.values {
			input := map[string]any{"kind": tc.kind, tc.field: value}
			if tc.kind == "QUANTITY" {
				input["unitCode"] = "M"
			}
			if err := catalogSchema(t, tc.schema).VisitJSON(input); err != nil {
				t.Fatalf("%s accepted canonical %q: %v", tc.schema, value, err)
			}
		}
	}
	for _, tc := range []struct {
		schema string
		kind   string
		values []string
	}{
		{"CatalogIntegerValue", "INTEGER", []string{"-0", "01"}},
		{"CatalogDecimalValue", "DECIMAL", []string{"-0", "01", "1.0", "1e3"}},
		{"CatalogQuantityValue", "QUANTITY", []string{"-0", "01", "1.0", "1e3"}},
	} {
		for _, value := range tc.values {
			input := map[string]any{"kind": tc.kind, "value": value}
			if tc.kind == "QUANTITY" {
				input["unitCode"] = "M"
			}
			if err := catalogSchema(t, tc.schema).VisitJSON(input); err == nil {
				t.Fatalf("%s accepted noncanonical %q", tc.schema, value)
			}
		}
	}
}

func TestCatalogOpenAPINumericFieldsRejectLineEndings(t *testing.T) {
	fields := []struct {
		name   string
		schema string
		input  func(string) any
	}{
		{"integer value", "CatalogIntegerValue", func(value string) any { return map[string]any{"kind": "INTEGER", "value": value} }},
		{"decimal value", "CatalogDecimalValue", func(value string) any { return map[string]any{"kind": "DECIMAL", "value": value} }},
		{"quantity value", "CatalogQuantityValue", func(value string) any { return map[string]any{"kind": "QUANTITY", "value": value, "unitCode": "M"} }},
		{"record id", "CatalogRecord", func(value string) any {
			return map[string]any{"kind": "K", "id": value, "revision": "1", "active": true, "values": map[string]any{}, "rules": []any{}}
		}},
		{"record revision", "CatalogRecord", func(value string) any {
			return map[string]any{"kind": "K", "id": "1", "revision": value, "active": true, "values": map[string]any{}, "rules": []any{}}
		}},
		{"reference id", "CatalogReference", func(value string) any { return map[string]any{"kind": "K", "id": value, "code": ""} }},
	}
	for _, ending := range []string{"\n", "\r", "\r\n"} {
		for _, field := range fields {
			t.Run(field.name, func(t *testing.T) {
				if err := catalogSchema(t, field.schema).VisitJSON(field.input("1" + ending)); err == nil {
					t.Fatalf("%s accepted line ending %q", field.name, ending)
				}
			})
		}
	}
}

func TestCatalogOpenAPIValueVariantsAreClosed(t *testing.T) {
	variants := []struct {
		schema string
		value  map[string]any
	}{
		{"CatalogTextValue", map[string]any{"kind": "TEXT", "value": "x"}},
		{"CatalogCodeValue", map[string]any{"kind": "CODE", "value": "x"}},
		{"CatalogBooleanValue", map[string]any{"kind": "BOOLEAN", "value": true}},
		{"CatalogIntegerValue", map[string]any{"kind": "INTEGER", "value": "1"}},
		{"CatalogDecimalValue", map[string]any{"kind": "DECIMAL", "value": "0.1"}},
		{"CatalogQuantityValue", map[string]any{"kind": "QUANTITY", "value": "0.1", "unitCode": "M"}},
		{"CatalogReferenceValue", map[string]any{"kind": "REFERENCE", "reference": map[string]any{"kind": "K", "id": "1", "code": ""}}},
		{"CatalogEnumValue", map[string]any{"kind": "ENUM", "value": "x"}},
		{"CatalogStringListValue", map[string]any{"kind": "STRING_LIST", "values": []any{}}},
		{"CatalogControlledOptionValue", map[string]any{"kind": "CONTROLLED_OPTION", "value": "x"}},
		{"CatalogNotApplicableValue", map[string]any{"kind": "NOT_APPLICABLE"}},
	}
	for _, variant := range variants {
		t.Run(variant.schema, func(t *testing.T) {
			variant.value["extra"] = true
			if err := catalogSchema(t, variant.schema).VisitJSON(variant.value); err == nil {
				t.Fatal("extra property matched variant")
			}
		})
	}
	nestedExtra := map[string]any{"kind": "REFERENCE", "reference": map[string]any{"kind": "K", "id": "1", "code": "", "extra": true}}
	for _, schema := range []string{"CatalogReferenceValue", "CatalogValue"} {
		if err := catalogSchema(t, schema).VisitJSON(nestedExtra); err == nil {
			t.Fatalf("%s accepted extra reference property", schema)
		}
	}
}

func catalogSchema(t *testing.T, name string) *openapi3.Schema {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromData(catalogOpenAPI)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return doc.Components.Schemas[name].Value
}

func validateSchemaJSON(t *testing.T, schema *openapi3.Schema, value any) {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var input any
	if err := json.Unmarshal(body, &input); err != nil {
		t.Fatal(err)
	}
	if err := schema.VisitJSON(input); err != nil {
		t.Fatal(err)
	}
}
