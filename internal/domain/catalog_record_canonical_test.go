package domain

import "testing"

func TestCanonicalizeCatalogRecordCodes(t *testing.T) {
	kind := CatalogKind{Fields: []FieldDescriptor{
		{Name: "code", Kind: FieldCode},
		{Name: "name", Kind: FieldText},
	}}
	original := CatalogRecord{Values: map[string]CatalogValue{
		"code": {Text: "  conductores  "},
		"name": {Text: "Conductores"},
	}}

	got := CanonicalizeCatalogRecordCodes(kind, original)

	if got.Values["code"].Text != "CONDUCTORES" {
		t.Fatalf("code = %q, want CONDUCTORES", got.Values["code"].Text)
	}
	if got.Values["name"].Text != "Conductores" {
		t.Fatalf("name field must not be canonicalized: got %q", got.Values["name"].Text)
	}
	if original.Values["code"].Text != "  conductores  " {
		t.Fatalf("original record was mutated: %q", original.Values["code"].Text)
	}
}

func TestCanonicalizeCatalogRecordCodesMissingCodeValue(t *testing.T) {
	kind := CatalogKind{Fields: []FieldDescriptor{{Name: "code", Kind: FieldCode}}}
	got := CanonicalizeCatalogRecordCodes(kind, CatalogRecord{})
	if got.Values != nil {
		t.Fatalf("expected nil Values to stay nil, got %#v", got.Values)
	}
}
