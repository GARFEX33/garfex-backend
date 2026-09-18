package domain

import (
	"errors"
	"testing"
)

// TestEffectiveAttributesForOrdersByPositionThenDeclaration covers the core
// contract: CABLE's insulation/gauge/color have configured PresentationField
// positions (1,2,3) and come first in that order; conductor_material and
// voltage have no configured position and follow in AttributesFor's own
// declaration order.
func TestEffectiveAttributesForOrdersByPositionThenDeclaration(t *testing.T) {
	catalog := SeedResourceCatalog()
	attributes, err := catalog.EffectiveAttributesFor(conductoresScope, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"insulation", "gauge", "color", "conductor_material", "voltage"}
	if len(attributes) != len(want) {
		t.Fatalf("EffectiveAttributesFor() returned %d attributes, want %d", len(attributes), len(want))
	}
	for i, code := range want {
		if attributes[i].Attribute.Definition.Code != code {
			t.Fatalf("EffectiveAttributesFor()[%d].Attribute.Definition.Code = %q, want %q", i, attributes[i].Attribute.Definition.Code, code)
		}
	}
}

// TestEffectiveAttributesForResolvesPresentationPositionAndSource covers
// Position/HasPosition and Source resolution: insulation is TYPE-scoped
// (CABLE) with configured position 1, conductor_material has no configured
// PresentationField at all.
func TestEffectiveAttributesForResolvesPresentationPositionAndSource(t *testing.T) {
	catalog := SeedResourceCatalog()
	byCode := mustEffectiveByCode(t, catalog, conductoresScope, nil)

	insulation := byCode["insulation"]
	if !insulation.HasPosition || insulation.Position != 1 {
		t.Fatalf("insulation Position/HasPosition = %d/%v, want 1/true", insulation.Position, insulation.HasPosition)
	}
	if insulation.SourceLevel != SourceLevelType || insulation.SourceCode != "CABLE" {
		t.Fatalf("insulation Source = %s/%s, want %s/CABLE", insulation.SourceLevel, insulation.SourceCode, SourceLevelType)
	}

	conductorMaterial := byCode["conductor_material"]
	if conductorMaterial.HasPosition {
		t.Fatalf("conductor_material HasPosition = true, want false (no PresentationField configured)")
	}
}

// TestEffectiveAttributesForIgnoresInactivePresentationFieldPosition covers
// the deactivate-a-PRESENTACION contract: once a PresentationField's own
// Active flag is false, EffectiveAttributesFor must stop treating its
// attribute as positioned (HasPosition/Position), the same way every other
// catalog kind's Active flag turns off runtime participation without a
// physical delete.
func TestEffectiveAttributesForIgnoresInactivePresentationFieldPosition(t *testing.T) {
	catalog := SeedResourceCatalog()
	deactivateInsulationPresentationField(t, &catalog)

	byCode := mustEffectiveByCode(t, catalog, conductoresScope, nil)
	insulation := byCode["insulation"]
	if insulation.HasPosition {
		t.Fatalf("insulation HasPosition = true after deactivating its PresentationField, want false")
	}
}

// deactivateInsulationPresentationField flips Active to false on CABLE's
// configured "insulation" PresentationField (position 1 in SeedResourceCatalog),
// failing the test if that fixture entry cannot be found.
func deactivateInsulationPresentationField(t *testing.T, catalog *ResourceCatalog) {
	t.Helper()
	for i, field := range catalog.PresentationFields {
		if field.TypeCode == "CABLE" && field.AttributeCode == "insulation" {
			catalog.PresentationFields[i].Active = false
			return
		}
	}
	t.Fatalf("fixture PresentationField for CABLE/insulation not found")
}

// TestEffectiveAttributesForResolvesConditionalModeAgainstCurrentValues
// covers CABLE's color: CONDITIONAL, defaulting to REQUIRED with no known
// values (ResourceAttribute.Effective's own unmatched-CONDITIONAL default),
// then FORBIDDEN+NotApplicable once insulation=DESNUDO is known — the exact
// scenario a frontend "evaluate" call must resolve correctly.
func TestEffectiveAttributesForResolvesConditionalModeAgainstCurrentValues(t *testing.T) {
	catalog := SeedResourceCatalog()

	baseline := mustEffectiveByCode(t, catalog, conductoresScope, nil)
	if mode := baseline["color"].EffectiveMode; mode != ModeRequired {
		t.Fatalf("color EffectiveMode with no current values = %q, want %q", mode, ModeRequired)
	}

	desnudo := mustEffectiveByCode(t, catalog, conductoresScope, []ResourceAttributeValue{OptionValue("insulation", "DESNUDO")})
	color := desnudo["color"]
	if color.EffectiveMode != ModeForbidden || !color.NotApplicable {
		t.Fatalf("color EffectiveMode/NotApplicable with insulation=DESNUDO = %q/%v, want %q/true", color.EffectiveMode, color.NotApplicable, ModeForbidden)
	}
	voltage := desnudo["voltage"]
	if voltage.EffectiveMode != ModeForbidden || !voltage.NotApplicable {
		t.Fatalf("voltage EffectiveMode/NotApplicable with insulation=DESNUDO = %q/%v, want %q/true", voltage.EffectiveMode, voltage.NotApplicable, ModeForbidden)
	}
}

// TestEffectiveAttributesForRejectsValueTypeMismatch reproduces the exact bug
// a frontend integration hit: sending insulation as CONTROLLED_TEXT (what a
// client's Value{Kind:"TEXT"} decodes to) against an attribute whose
// definition is CONTROLLED_OPTION must be rejected the same way NewResource
// already rejects it (canonicalValue's type check) — never silently accepted
// with the value's OptionCode left empty, which would make every rule
// referencing it silently fail to match.
func TestEffectiveAttributesForRejectsValueTypeMismatch(t *testing.T) {
	catalog := SeedResourceCatalog()
	mismatched := ResourceAttributeValue{AttributeCode: "insulation", Type: ValueTypeControlledText, Text: "DESNUDO"}

	_, err := catalog.EffectiveAttributesFor(conductoresScope, []ResourceAttributeValue{mismatched})
	if !errors.Is(err, ErrResourceValidation) {
		t.Fatalf("EffectiveAttributesFor(mismatched type) error = %v, want ErrResourceValidation", err)
	}
}

// TestEffectiveAttributesForRejectsUnknownOrRepeatedAttribute mirrors
// NewResource's own two other value-shape guards, applied identically here.
func TestEffectiveAttributesForRejectsUnknownOrRepeatedAttribute(t *testing.T) {
	catalog := SeedResourceCatalog()

	_, err := catalog.EffectiveAttributesFor(conductoresScope, []ResourceAttributeValue{OptionValue("nope", "X")})
	if !errors.Is(err, ErrResourceValidation) {
		t.Fatalf("EffectiveAttributesFor(unknown attribute) error = %v, want ErrResourceValidation", err)
	}

	_, err = catalog.EffectiveAttributesFor(conductoresScope, []ResourceAttributeValue{
		OptionValue("insulation", "THW"), OptionValue("insulation", "THHN"),
	})
	if !errors.Is(err, ErrResourceValidation) {
		t.Fatalf("EffectiveAttributesFor(repeated attribute) error = %v, want ErrResourceValidation", err)
	}
}

// TestEffectiveAttributesForResolvesOptionsNarrowedByRelations covers
// TUBERIA's diameter_inch/diameter_mm relation (design D3): with no current
// values every catalog option for diameter_mm is returned unconstrained;
// once diameter_inch is chosen, diameter_mm's Options narrow to the one
// related value — reusing ResourceCatalog.ValidOptions exactly as
// NewResource does, never re-implemented here.
func TestEffectiveAttributesForResolvesOptionsNarrowedByRelations(t *testing.T) {
	catalog := SeedResourceCatalog()

	unconstrained := mustEffectiveByCode(t, catalog, canalizacionesScope, nil)
	if got := len(unconstrained["diameter_mm"].Options); got != 9 {
		t.Fatalf("diameter_mm unconstrained Options count = %d, want 9", got)
	}

	narrowed := mustEffectiveByCode(t, catalog, canalizacionesScope, []ResourceAttributeValue{OptionValue("diameter_inch", `1/2"`)})
	options := narrowed["diameter_mm"].Options
	if len(options) != 1 || options[0].Code != "13 mm" {
		t.Fatalf("diameter_mm narrowed Options = %+v, want exactly one option 13 mm", options)
	}
}

// TestEffectiveAttributesForUnknownScopeReturnsEmpty mirrors AttributesFor's
// own read-query contract: an unresolvable scope yields an empty result, not
// an error — validation is NewResource's (and, for values, EffectiveAttributesFor's own) job.
func TestEffectiveAttributesForUnknownScopeReturnsEmpty(t *testing.T) {
	catalog := SeedResourceCatalog()
	got, err := catalog.EffectiveAttributesFor(ResourceScope{ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "NOPE"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("EffectiveAttributesFor(unknown type) = %v, want empty", got)
	}
}

func mustEffectiveByCode(t *testing.T, catalog ResourceCatalog, scope ResourceScope, current []ResourceAttributeValue) map[string]EffectiveAttribute {
	t.Helper()
	attributes, err := catalog.EffectiveAttributesFor(scope, current)
	if err != nil {
		t.Fatalf("EffectiveAttributesFor() unexpected error: %v", err)
	}
	byCode := map[string]EffectiveAttribute{}
	for _, attribute := range attributes {
		byCode[attribute.Attribute.Definition.Code] = attribute
	}
	return byCode
}
