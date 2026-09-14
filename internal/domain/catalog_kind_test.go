package domain

import "testing"

// RED (task 3.1): CatalogRegistry does not exist yet — every assertion below
// references NewCatalogRegistry/CatalogKind, which fails to compile until
// catalog_kind.go's GREEN lands. Scenarios per design's Testing Strategy for
// catalog_kind_test.go: every kind has Spanish Singular/Plural; IdentityFields
// name real fields; every RefKind/ParentKind resolves to a registered kind;
// every identity-bearing kind (i.e. every kind except the pure junction/
// relation kinds OptionRelation/UnitPolicy/AttributeBinding/
// PresentationField — see design §6's own Código-immutability probe list,
// which likewise never names those four) carries exactly one "code" field
// marked ImmutableOnceReferenced.

// junctionKinds are the CatalogKind codes design §6's Código-immutability
// probe table never lists — they are identified by composite references
// (e.g. class+family+unit), not an independent Código, so they are exempt
// from the "must carry an immutable code field" rule below.
func junctionKinds() map[CatalogKindCode]bool {
	return map[CatalogKindCode]bool{
		KindOptionRelation:    true,
		KindUnitPolicy:        true,
		KindAttributeBinding:  true,
		KindPresentationField: true,
	}
}

func TestCatalogRegistry_HasElevenKinds(t *testing.T) {
	registry := NewCatalogRegistry()
	kinds := registry.Kinds()
	if len(kinds) != 11 {
		t.Fatalf("NewCatalogRegistry().Kinds() returned %d kinds, want 11", len(kinds))
	}
}

func TestCatalogRegistry_EveryKindHasSpanishLabels(t *testing.T) {
	registry := NewCatalogRegistry()
	for _, kind := range registry.Kinds() {
		if kind.Singular == "" {
			t.Errorf("kind %q has an empty Singular label", kind.Code)
		}
		if kind.Plural == "" {
			t.Errorf("kind %q has an empty Plural label", kind.Code)
		}
		if kind.Singular == kind.Plural {
			t.Errorf("kind %q Singular == Plural (%q) — Spanish catalog labels must actually pluralize", kind.Code, kind.Singular)
		}
	}
}

func TestCatalogRegistry_IdentityFieldsNameRealFields(t *testing.T) {
	registry := NewCatalogRegistry()
	for _, kind := range registry.Kinds() {
		fieldByName := map[string]bool{}
		for _, field := range kind.Fields {
			fieldByName[field.Name] = true
		}
		if len(kind.IdentityFields) == 0 {
			t.Errorf("kind %q has no IdentityFields", kind.Code)
		}
		for _, name := range kind.IdentityFields {
			if !fieldByName[name] {
				t.Errorf("kind %q.IdentityFields names %q, which is not one of its own Fields", kind.Code, name)
			}
		}
	}
}

func TestCatalogRegistry_EveryRefKindAndParentKindResolves(t *testing.T) {
	registry := NewCatalogRegistry()
	for _, kind := range registry.Kinds() {
		if kind.ParentKind != "" {
			if _, ok := registry.Kind(kind.ParentKind); !ok {
				t.Errorf("kind %q.ParentKind = %q does not resolve to a registered kind", kind.Code, kind.ParentKind)
			}
		}
		for _, field := range kind.Fields {
			if field.Kind != FieldRef {
				continue
			}
			if _, ok := registry.Kind(field.RefKind); !ok {
				t.Errorf("kind %q field %q.RefKind = %q does not resolve to a registered kind", kind.Code, field.Name, field.RefKind)
			}
		}
		for _, child := range kind.Children {
			if _, ok := registry.Kind(child.Kind); !ok {
				t.Errorf("kind %q child relation names unregistered kind %q", kind.Code, child.Kind)
			}
		}
	}
}

func TestCatalogRegistry_EveryNonJunctionKindHasAnImmutableCodeField(t *testing.T) {
	registry := NewCatalogRegistry()
	junctions := junctionKinds()
	for _, kind := range registry.Kinds() {
		if junctions[kind.Code] {
			continue
		}
		var codeFields int
		for _, field := range kind.Fields {
			if field.Name == "code" {
				codeFields++
				if field.Kind != FieldCode {
					t.Errorf("kind %q field \"code\" has Kind = %v, want FieldCode", kind.Code, field.Kind)
				}
				if field.Immutable != ImmutableOnceReferenced {
					t.Errorf("kind %q field \"code\" has Immutable = %v, want ImmutableOnceReferenced", kind.Code, field.Immutable)
				}
			}
		}
		if codeFields != 1 {
			t.Errorf("kind %q has %d fields named \"code\", want exactly 1", kind.Code, codeFields)
		}
	}
}

// TestCatalogRegistry_JunctionKindsHaveNoCodeField triangulates the previous
// test: the four exempted kinds must genuinely lack a "code" field (proving
// the exemption isn't hiding a bug rather than a real structural fact).
func TestCatalogRegistry_JunctionKindsHaveNoCodeField(t *testing.T) {
	registry := NewCatalogRegistry()
	for code := range junctionKinds() {
		kind, ok := registry.Kind(code)
		if !ok {
			t.Fatalf("junction kind %q is not registered", code)
		}
		for _, field := range kind.Fields {
			if field.Name == "code" {
				t.Errorf("junction kind %q unexpectedly has a \"code\" field", kind.Code)
			}
		}
	}
}

func TestCatalogRegistry_KindLooksUpByCode(t *testing.T) {
	registry := NewCatalogRegistry()
	kind, ok := registry.Kind(KindFamily)
	if !ok {
		t.Fatalf("Kind(KindFamily) ok = false, want true")
	}
	if kind.Singular != "Familia" {
		t.Fatalf("Kind(KindFamily).Singular = %q, want %q", kind.Singular, "Familia")
	}

	if _, ok := registry.Kind(CatalogKindCode("NOPE")); ok {
		t.Fatalf("Kind(NOPE) ok = true, want false")
	}
}

// TestCatalogRegistry_KindsReturnsAnIndependentCopy proves Kinds() cannot be
// used to mutate the registry's internal state (a defensive-copy contract,
// mirroring ApplyCatalogMutation's own copy-on-write discipline).
func TestCatalogRegistry_KindsReturnsAnIndependentCopy(t *testing.T) {
	registry := NewCatalogRegistry()
	kinds := registry.Kinds()
	kinds[0].Singular = "MUTATED"

	again := registry.Kinds()
	if again[0].Singular == "MUTATED" {
		t.Fatalf("mutating the slice returned by Kinds() leaked into the registry's own state")
	}
}

// TestCatalogRegistry_AplicabilidadModeEnumMatchesValidatedModes catches the
// APLICABILIDAD "mode" field's EnumValues drifting from the modes
// ApplyCatalogMutation actually accepts (validAttributeMode in
// catalog_mutation.go): a descriptor missing a valid mode silently hides it
// from every /docs-driven UI even though the backend allows it.
func TestCatalogRegistry_AplicabilidadModeEnumMatchesValidatedModes(t *testing.T) {
	registry := NewCatalogRegistry()
	kind, ok := registry.Kind(KindAttributeBinding)
	if !ok {
		t.Fatalf("KindAttributeBinding is not registered")
	}
	var modeField *FieldDescriptor
	for i, field := range kind.Fields {
		if field.Name == "mode" {
			modeField = &kind.Fields[i]
		}
	}
	if modeField == nil {
		t.Fatalf("KindAttributeBinding has no %q field", "mode")
	}
	enumValues := map[string]bool{}
	for _, v := range modeField.EnumValues {
		enumValues[v.Value] = true
	}
	for _, mode := range []AttributeMode{ModeRequired, ModeOptional, ModeConditional, ModeForbidden} {
		if !enumValues[string(mode)] {
			t.Errorf("mode field EnumValues %+v is missing %q, which validAttributeMode accepts", modeField.EnumValues, mode)
		}
	}
}

func TestCatalogKindRegistry_AllRegisteredKindsAreLifecycleCapable(t *testing.T) {
	want := []CatalogKindCode{
		KindClass, KindFamily, KindType, KindAttributeDefinition, KindOptionSet,
		KindOption, KindOptionRelation, KindUnit, KindUnitPolicy, KindAttributeBinding,
		KindPresentationField,
	}
	registry := NewCatalogRegistry()
	if got := len(registry.Kinds()); got != len(want) {
		t.Fatalf("registry has %d kinds, want exactly %d", got, len(want))
	}
	for _, code := range want {
		kind, ok := registry.Kind(code)
		if !ok {
			t.Fatalf("expected registered kind %q", code)
		}
		if !kind.SoftDelete {
			t.Errorf("kind %q is not lifecycle-capable", code)
		}
	}
}
