package domain

import "sort"

// SourceLevel names which catalog level a resolved attribute's binding comes
// from: SourceLevelFamily for a family-shared ResourceAttribute (empty
// TypeCode), SourceLevelType for one bound to the exact ResourceType.
const (
	SourceLevelFamily = "FAMILY"
	SourceLevelType   = "TYPE"
)

// EffectiveAttribute is the resolved, UI-ready view of one ResourceAttribute
// for a specific ResourceScope: which catalog level its binding comes from,
// its configured PresentationField position (if any), its mode/identity/
// applicability resolved against already-known values (ResourceAttribute.
// Effective), and its allowed Options already narrowed by
// AttributeOptionRelation (ResourceCatalog.ValidOptions) — the same
// resolution NewResource applies internally, exposed so a caller never has
// to re-implement inheritance, rule evaluation, or relation narrowing.
type EffectiveAttribute struct {
	Attribute            ResourceAttribute
	EffectiveMode        AttributeMode
	IdentityParticipates bool
	NotApplicable        bool
	Position             int
	HasPosition          bool
	SourceLevel          string
	SourceCode           string
	Options              []AttributeOption
}

// EffectiveAttributesFor resolves every ResourceAttribute applicable to s
// (both family-shared and type-specific — see AttributesFor), combined with
// its configured PresentationField position and its mode/identity/
// applicability resolved against current (nil/empty yields the scope's
// static baseline: an unmatched CONDITIONAL attribute defaults to REQUIRED —
// see ResourceAttribute.Effective).
//
// current is canonicalized against each attribute's own definition exactly
// as NewResource does (canonicalValue) before evaluation: an unknown
// attribute code, a repeated attribute code, or a value whose Type does not
// match its attribute's Definition.ValueType is rejected with
// ErrResourceValidation, never silently accepted with a value that can
// never match a rule (e.g. a CONTROLLED_TEXT value for a CONTROLLED_OPTION
// attribute would otherwise carry no OptionCode and silently fail every
// rule comparison).
//
// Attributes with a configured position come first, ordered by that
// position; the rest follow in the catalog's own declaration order
// (AttributesFor's order) — never an automatic/alphabetical reordering a UI
// didn't ask for. An unresolvable scope yields an empty result, not an
// error, mirroring AttributesFor's own read-query contract.
func (c ResourceCatalog) EffectiveAttributesFor(s ResourceScope, current []ResourceAttributeValue) ([]EffectiveAttribute, error) {
	s = s.canonicalize()
	attributes := c.AttributesFor(s)
	byCode := map[string]ResourceAttribute{}
	for _, attribute := range attributes {
		byCode[canonicalAttribute(attribute.Definition.Code)] = attribute
	}
	canonicalCurrent := make([]ResourceAttributeValue, 0, len(current))
	seen := map[string]bool{}
	for _, value := range current {
		code := canonicalAttribute(value.AttributeCode)
		if seen[code] {
			return nil, validation("attribute %q is repeated", code)
		}
		seen[code] = true
		attribute, ok := byCode[code]
		if !ok {
			return nil, validation("attribute %q is not defined for family %q", code, s.FamilyCode)
		}
		canonicalValue, err := c.canonicalValue(attribute, value)
		if err != nil {
			return nil, err
		}
		canonicalCurrent = append(canonicalCurrent, canonicalValue)
	}
	positions := map[string]int{}
	for _, field := range c.presentationFields(s) {
		positions[canonicalAttribute(field.AttributeCode)] = field.Position
	}
	result := make([]EffectiveAttribute, len(attributes))
	for i, attribute := range attributes {
		mode, participates, notApplicable := attribute.Effective(canonicalCurrent)
		level, code := SourceLevelFamily, s.FamilyCode
		if attribute.TypeCode != "" {
			level, code = SourceLevelType, attribute.TypeCode
		}
		position, hasPosition := positions[canonicalAttribute(attribute.Definition.Code)]
		result[i] = EffectiveAttribute{
			Attribute: attribute, EffectiveMode: mode, IdentityParticipates: participates,
			NotApplicable: notApplicable, Position: position, HasPosition: hasPosition,
			SourceLevel: level, SourceCode: code, Options: c.ValidOptions(attribute, canonicalCurrent),
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].HasPosition != result[j].HasPosition {
			return result[i].HasPosition
		}
		return result[i].HasPosition && result[i].Position < result[j].Position
	})
	return result, nil
}
