package domain

import (
	"fmt"
	"testing"
)

// Task 2b.3: adapted from the pre-rename material_presentation_test.go.
// PresentationField now carries the full ClassCode+FamilyCode+TypeCode
// triple (design R2) and every fixture Resource literal needs ClassCode set
// or Describe's internal resourceType(scope) lookup misses (see PR2a's
// apply-progress notes).

// TestDescribeCableComposesInsulationGaugeColor covers D1's core case:
// CABLE's PresentationFields (insulation, gauge, color, in that order) are
// composed after the ResourceType name, while attributes NOT configured for
// presentation (conductor_material, voltage) are silently excluded — never
// an automatic dump of every attribute. Color keeps its catalog-canonical
// uppercase form ("BLANCO"); no casing/grammar layer is applied.
func TestDescribeCableComposesInsulationGaugeColor(t *testing.T) {
	catalog := SeedResourceCatalog()
	resource, err := NewResource(catalog, conductoresScope, "M", []ResourceAttributeValue{
		OptionValue("conductor_material", "COBRE"),
		OptionValue("gauge", "12 AWG"),
		OptionValue("insulation", "THHN"),
		OptionValue("color", "BLANCO"),
		OptionValue("voltage", "600 V"),
	})
	if err != nil {
		t.Fatalf("NewResource() error = %v", err)
	}
	want := "Cable THHN 12 AWG BLANCO"
	if got := catalog.Describe(resource); got != want {
		t.Fatalf("Describe() = %q, want %q", got, want)
	}
}

// TestDescribeTuberiaComposesTipoAndDiameter covers TUBERIA's
// PresentationFields (tipo, diameter_inch), using the ResourceType's actual
// catalog Name ("Tubería").
func TestDescribeTuberiaComposesTipoAndDiameter(t *testing.T) {
	catalog := SeedResourceCatalog()
	resource, err := NewResource(catalog, canalizacionesScope, "PZA", []ResourceAttributeValue{
		OptionValue("tipo", "CONDUIT PARED DELGADA"),
		OptionValue("diameter_inch", `1/2"`),
		OptionValue("diameter_mm", "13 mm"),
	})
	if err != nil {
		t.Fatalf("NewResource() error = %v", err)
	}
	want := `Tubería CONDUIT PARED DELGADA 1/2"`
	if got := catalog.Describe(resource); got != want {
		t.Fatalf("Describe() = %q, want %q", got, want)
	}
}

// TestDescribeSkipsFieldMissingEntirely covers a PresentationField whose
// attribute is simply absent from Attributes (e.g. a DESNUDO conductor,
// where color/voltage are structurally forbidden and never set) — it is
// skipped silently, no blank segment.
func TestDescribeSkipsFieldMissingEntirely(t *testing.T) {
	catalog := SeedResourceCatalog()
	resource, err := NewResource(catalog, conductoresScope, "M", []ResourceAttributeValue{
		OptionValue("conductor_material", "COBRE"),
		OptionValue("gauge", "12 AWG"),
		OptionValue("insulation", "DESNUDO"),
	})
	if err != nil {
		t.Fatalf("NewResource() error = %v", err)
	}
	want := "Cable DESNUDO 12 AWG"
	if got := catalog.Describe(resource); got != want {
		t.Fatalf("Describe() = %q, want %q", got, want)
	}
}

// TestDescribeSkipsFieldMarkedNotApplicable covers a PresentationField whose
// attribute IS present but carries the NotApplicableText sentinel (as a real
// Resource fetched from the repository would for a DESNUDO conductor's
// color) — also skipped silently, no blank segment, no error.
func TestDescribeSkipsFieldMarkedNotApplicable(t *testing.T) {
	catalog := SeedResourceCatalog()
	resource := Resource{
		ClassCode:   "MATERIAL",
		FamilyCode:  "CONDUCTORES",
		TypeCode:    "CABLE",
		NaturalUnit: "M",
		Attributes: []ResourceAttributeValue{
			OptionValue("conductor_material", "COBRE"),
			OptionValue("gauge", "12 AWG"),
			OptionValue("insulation", "DESNUDO"),
			{AttributeCode: "color", Type: ValueTypeControlledOption, Text: NotApplicableText},
		},
	}
	want := "Cable DESNUDO 12 AWG"
	if got := catalog.Describe(resource); got != want {
		t.Fatalf("Describe() = %q, want %q", got, want)
	}
}

// TestDescribeSkipsInactivePresentationField covers the deactivate-a-
// PRESENTACION contract: a PresentationField whose own Active flag is false
// must stop composing the name, the same way every other catalog kind's
// Active flag turns off runtime participation without a physical delete.
func TestDescribeSkipsInactivePresentationField(t *testing.T) {
	catalog := SeedResourceCatalog()
	deactivateInsulationPresentationField(t, &catalog)

	resource, err := NewResource(catalog, conductoresScope, "M", []ResourceAttributeValue{
		OptionValue("conductor_material", "COBRE"),
		OptionValue("gauge", "12 AWG"),
		OptionValue("insulation", "THHN"),
		OptionValue("color", "BLANCO"),
		OptionValue("voltage", "600 V"),
	})
	if err != nil {
		t.Fatalf("NewResource() error = %v", err)
	}
	want := "Cable 12 AWG BLANCO"
	if got := catalog.Describe(resource); got != want {
		t.Fatalf("Describe() = %q, want %q (insulation's PresentationField is inactive)", got, want)
	}
}

// TestDescribeWithNoPresentationFieldsReturnsOnlyName covers D1's explicit
// safe degradation: a ResourceType with zero matching PresentationFields
// entries returns exactly its own Name, never a fallback to an improvised
// technical dump.
func TestDescribeWithNoPresentationFieldsReturnsOnlyName(t *testing.T) {
	catalog := ResourceCatalog{
		Types: []ResourceType{{ClassCode: "MATERIAL", FamilyCode: "WIDGETS", Code: "GADGET", Name: "Gadget"}},
	}
	resource := Resource{ClassCode: "MATERIAL", FamilyCode: "WIDGETS", TypeCode: "GADGET", NaturalUnit: "PZA"}
	want := "Gadget"
	if got := catalog.Describe(resource); got != want {
		t.Fatalf("Describe() = %q, want %q", got, want)
	}
}

// TestDescribeUnknownTypeReturnsEmpty covers a Resource whose TypeCode
// cannot be resolved in the catalog.
func TestDescribeUnknownTypeReturnsEmpty(t *testing.T) {
	catalog := SeedResourceCatalog()
	resource := Resource{ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "NOPE", NaturalUnit: "M"}
	if got := catalog.Describe(resource); got != "" {
		t.Fatalf("Describe() = %q, want %q", got, "")
	}
}

// TestPresentationFieldsReferenceApplicableResourceAttributes is a
// catalog-self-consistency invariant: every PresentationFields entry must
// reference a defined ResourceType, and its AttributeCode must correspond
// to a ResourceAttribute that is actually applicable to that entry's
// class+family+type (a row whose TypeCode is either "" (shared) or exactly
// that entry's TypeCode).
func TestPresentationFieldsReferenceApplicableResourceAttributes(t *testing.T) {
	catalog := SeedResourceCatalog()
	for _, field := range catalog.PresentationFields {
		found := false
		for _, resourceType := range catalog.Types {
			if resourceType.ClassCode == field.ClassCode && resourceType.FamilyCode == field.FamilyCode && resourceType.Code == field.TypeCode {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("PresentationFields entry references unknown type (%q, %q, %q)", field.ClassCode, field.FamilyCode, field.TypeCode)
		}
		applicable := false
		for _, attribute := range catalog.Attributes {
			if attribute.ClassCode != field.ClassCode || attribute.FamilyCode != field.FamilyCode || attribute.Definition.Code != field.AttributeCode {
				continue
			}
			if attribute.TypeCode == "" || attribute.TypeCode == field.TypeCode {
				applicable = true
				break
			}
		}
		if !applicable {
			t.Fatalf("PresentationFields entry (%q, %q, %q, %q) has no applicable ResourceAttribute", field.ClassCode, field.FamilyCode, field.TypeCode, field.AttributeCode)
		}
	}
}

// TestPresentationFieldsNoDuplicateAttributePerType is a
// catalog-self-consistency invariant: no two PresentationFields entries
// share the same (ClassCode, FamilyCode, TypeCode, AttributeCode) tuple.
func TestPresentationFieldsNoDuplicateAttributePerType(t *testing.T) {
	catalog := SeedResourceCatalog()
	seen := map[string]bool{}
	for _, field := range catalog.PresentationFields {
		key := field.ClassCode + "|" + field.FamilyCode + "|" + field.TypeCode + "|" + field.AttributeCode
		if seen[key] {
			t.Fatalf("duplicate PresentationFields (class, family, type, attribute) key %q", key)
		}
		seen[key] = true
	}
}

// TestPresentationFieldsNoDuplicatePositionPerType is a
// catalog-self-consistency invariant: no two PresentationFields entries
// share the same (ClassCode, FamilyCode, TypeCode, Position) tuple.
func TestPresentationFieldsNoDuplicatePositionPerType(t *testing.T) {
	catalog := SeedResourceCatalog()
	seen := map[string]bool{}
	for _, field := range catalog.PresentationFields {
		key := fmt.Sprintf("%s|%s|%s|%d", field.ClassCode, field.FamilyCode, field.TypeCode, field.Position)
		if seen[key] {
			t.Fatalf("duplicate PresentationFields (class, family, type, position) key %q", key)
		}
		seen[key] = true
	}
}
