package domain

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func orderFixture() (ResourceScope, AttributeOrderState) {
	scope := ResourceScope{ClassCode: "MAT", FamilyCode: "F", TypeCode: "T"}
	return scope, AttributeOrderState{TypeID: 10, Members: []AttributeOrderMember{
		{Key: AttributeOrderKey{SourceLevel: SourceLevelFamily, SourceCode: "F", CharacteristicCode: "size"}, ResourceAttributeID: 1},
		{Key: AttributeOrderKey{SourceLevel: SourceLevelType, SourceCode: "T", CharacteristicCode: "size"}, ResourceAttributeID: 2},
		{Key: AttributeOrderKey{SourceLevel: SourceLevelType, SourceCode: "T", CharacteristicCode: "color"}, ResourceAttributeID: 3},
	}}
}

func TestAttributeOrderOverlay(t *testing.T) {
	for _, tt := range []struct {
		name  string
		saved []int64
		want  []int
	}{
		{name: "baseline", want: []int{0, 1, 2}},
		{name: "survivors then new", saved: []int64{3, 99, 1}, want: []int{2, 0, 1}},
		{name: "recreated binding appends", saved: []int64{99, 3}, want: []int{2, 0, 1}},
		{name: "all stale saved", saved: []int64{98, 99}, want: []int{0, 1, 2}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			scope, state := orderFixture()
			state.SavedBindingIDs = tt.saved
			got, err := ResolveAttributeOrder(scope, state)
			if err != nil {
				t.Fatal(err)
			}
			for i, index := range tt.want {
				if got.OrderedAttributes[i] != state.Members[index].Key {
					t.Fatalf("order = %+v", got)
				}
			}
			if !strings.HasPrefix(got.OrderRevision, "v1:") {
				t.Fatalf("revision = %q", got.OrderRevision)
			}
			got.OrderedAttributes[0].SourceCode = "changed"
			if state.Members[tt.want[0]].Key.SourceCode == "changed" {
				t.Fatal("result aliases input")
			}
		})
	}
	scope, state := orderFixture()
	state.Members = nil
	got, err := ResolveAttributeOrder(scope, state)
	if err != nil || got.OrderedAttributes == nil || len(got.OrderedAttributes) != 0 {
		t.Fatalf("empty = %+v, %v", got, err)
	}
}

func TestAttributeOrderRejectsInvalidState(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*ResourceScope, *AttributeOrderState)
	}{
		{name: "missing type", change: func(s *ResourceScope, _ *AttributeOrderState) { s.TypeCode = " " }},
		{name: "missing class", change: func(s *ResourceScope, _ *AttributeOrderState) { s.ClassCode = "" }},
		{name: "missing family", change: func(s *ResourceScope, _ *AttributeOrderState) { s.FamilyCode = "" }},
		{name: "missing type incarnation", change: func(_ *ResourceScope, s *AttributeOrderState) { s.TypeID = 0 }},
		{name: "duplicate composite", change: func(_ *ResourceScope, s *AttributeOrderState) { s.Members[1].Key = s.Members[0].Key }},
		{name: "canonical duplicate", change: func(_ *ResourceScope, s *AttributeOrderState) {
			s.Members[1].Key = AttributeOrderKey{SourceLevel: " family ", SourceCode: " f ", CharacteristicCode: " SIZE "}
		}},
		{name: "duplicate incarnation", change: func(_ *ResourceScope, s *AttributeOrderState) { s.Members[1].ResourceAttributeID = 1 }},
		{name: "invalid incarnation", change: func(_ *ResourceScope, s *AttributeOrderState) { s.Members[0].ResourceAttributeID = 0 }},
		{name: "foreign source", change: func(_ *ResourceScope, s *AttributeOrderState) { s.Members[0].Key.SourceCode = "OTHER" }},
		{name: "unknown level", change: func(_ *ResourceScope, s *AttributeOrderState) { s.Members[0].Key.SourceLevel = "OTHER" }},
		{name: "empty characteristic", change: func(_ *ResourceScope, s *AttributeOrderState) { s.Members[0].Key.CharacteristicCode = " " }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			scope, state := orderFixture()
			tt.change(&scope, &state)
			if _, err := ResolveAttributeOrder(scope, state); !errors.Is(err, ErrResourceValidation) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestAttributeOrderPermutation(t *testing.T) {
	scope, state := orderFixture()
	snapshot, err := ResolveAttributeOrder(scope, state)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name    string
		indexes []int
		valid   bool
	}{
		{name: "full reverse", indexes: []int{2, 1, 0}, valid: true},
		{name: "same characteristic distinct sources", indexes: []int{0, 1, 2}, valid: true},
		{name: "missing", indexes: []int{0, 1}},
		{name: "duplicate", indexes: []int{0, 0, 2}},
		{name: "extra", indexes: []int{0, 1, 2, 0}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			keys := make([]AttributeOrderKey, 0, len(tt.indexes))
			for _, i := range tt.indexes {
				keys = append(keys, state.Members[i].Key)
			}
			got, err := snapshot.ValidatePermutation(keys)
			if (err == nil) != tt.valid {
				t.Fatalf("error = %v", err)
			}
			if tt.valid {
				got[0].SourceCode = "changed"
				if keys[0].SourceCode == "changed" {
					t.Fatal("permutation aliases input")
				}
			}
		})
	}
	keys := slices.Clone(snapshot.OrderedAttributes)
	keys[0].CharacteristicCode = " SIZE "
	got, err := snapshot.ValidatePermutation(keys)
	if err != nil || got[0] != snapshot.OrderedAttributes[0] {
		t.Fatalf("canonical keys = %+v, %v", got, err)
	}
	keys[0].SourceCode = "OTHER"
	if _, err := snapshot.ValidatePermutation(keys); !errors.Is(err, ErrResourceValidation) {
		t.Fatalf("foreign error = %v", err)
	}
}

func TestAttributeOrderRevision(t *testing.T) {
	scope, state := orderFixture()
	baseline, err := ResolveAttributeOrder(scope, state)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name   string
		change func(*ResourceScope, *AttributeOrderState)
	}{
		{name: "scope", change: func(s *ResourceScope, _ *AttributeOrderState) { s.ClassCode = "OTHER" }},
		{name: "type incarnation", change: func(_ *ResourceScope, s *AttributeOrderState) { s.TypeID++ }},
		{name: "head revision", change: func(_ *ResourceScope, s *AttributeOrderState) { s.HeadRevision++ }},
		{name: "binding recreated", change: func(_ *ResourceScope, s *AttributeOrderState) { s.Members[0].ResourceAttributeID = 100 }},
		{name: "binding revision", change: func(_ *ResourceScope, s *AttributeOrderState) { s.Members[0].Revision++ }},
		{name: "membership", change: func(_ *ResourceScope, s *AttributeOrderState) { s.Members = s.Members[:2] }},
		{name: "renamed eligible binding", change: func(_ *ResourceScope, s *AttributeOrderState) {
			s.Members[0].Key.CharacteristicCode = "width"
		}},
		{name: "order", change: func(_ *ResourceScope, s *AttributeOrderState) { s.SavedBindingIDs = []int64{3} }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			scope, state := orderFixture()
			tt.change(&scope, &state)
			got, err := ResolveAttributeOrder(scope, state)
			if err != nil {
				t.Fatal(err)
			}
			if got.OrderRevision == baseline.OrderRevision {
				t.Fatal("revision did not change")
			}
		})
	}
	scope.ClassCode = " mat "
	got, err := ResolveAttributeOrder(scope, state)
	if err != nil || got.OrderRevision != baseline.OrderRevision {
		t.Fatalf("unstable canonical revision: %+v, %v", got, err)
	}
}

func TestAttributeOrderPreservesPresentationAndIdentity(t *testing.T) {
	catalog := SeedResourceCatalog()
	resource, err := NewResource(catalog, conductoresScope, "M", []ResourceAttributeValue{
		OptionValue("conductor_material", "COBRE"), OptionValue("gauge", "12 AWG"),
		OptionValue("insulation", "THHN"), OptionValue("color", "BLANCO"), OptionValue("voltage", "600 V"),
	})
	if err != nil {
		t.Fatal(err)
	}
	before := cloneResourceCatalog(catalog)
	description := catalog.Describe(resource)
	effective, err := catalog.EffectiveAttributesFor(conductoresScope, nil)
	if err != nil {
		t.Fatal(err)
	}
	state := AttributeOrderState{TypeID: 1}
	for i, a := range effective {
		state.Members = append(state.Members, AttributeOrderMember{
			Key:                 AttributeOrderKey{SourceLevel: a.SourceLevel, SourceCode: a.SourceCode, CharacteristicCode: a.Attribute.Definition.Code},
			ResourceAttributeID: int64(i + 1),
		})
		state.SavedBindingIDs = append([]int64{int64(i + 1)}, state.SavedBindingIDs...)
	}
	snapshot, err := ResolveAttributeOrder(conductoresScope, state)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.OrderedAttributes[0] != state.Members[len(state.Members)-1].Key {
		t.Fatal("visual order unchanged")
	}
	after, err := catalog.EffectiveAttributesFor(conductoresScope, nil)
	if err != nil {
		t.Fatal(err)
	}
	rebuilt, err := NewResource(catalog, conductoresScope, "M", resource.Attributes)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, catalog) || !reflect.DeepEqual(effective, after) {
		t.Fatal("catalog/presentation metadata mutated")
	}
	if catalog.Describe(resource) != description || !reflect.DeepEqual(resource, rebuilt) {
		t.Fatal("description/identity changed")
	}
}
