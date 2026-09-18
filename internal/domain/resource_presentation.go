package domain

import (
	"sort"
	"strings"
)

// Describe resolves the canonical, human-readable presentation of r: its
// ResourceType's name followed by its catalog-configured
// PresentationFields, in order. It never falls back to listing every
// attribute — a ResourceType with no PresentationFields configured degrades
// to just its own name (an intentionally incomplete-but-safe result, not an
// improvised technical dump), and a Resource whose ResourceType cannot be
// resolved in the catalog returns "".
func (c ResourceCatalog) Describe(r Resource) string {
	scope := ResourceScope{ClassCode: r.ClassCode, FamilyCode: r.FamilyCode, TypeCode: r.TypeCode}.canonicalize()
	resourceType, ok := c.resourceType(scope)
	if !ok {
		return ""
	}
	fields := c.presentationFields(scope)
	if len(fields) == 0 {
		return resourceType.Name
	}
	byCode := map[string]ResourceAttributeValue{}
	for _, value := range r.Attributes {
		byCode[canonicalAttribute(value.AttributeCode)] = value
	}
	parts := []string{resourceType.Name}
	for _, field := range fields {
		value, present := byCode[canonicalAttribute(field.AttributeCode)]
		if !present || value.Text == NotApplicableText {
			continue
		}
		parts = append(parts, value.canonical(c.definitionFor(field.AttributeCode)))
	}
	return strings.Join(parts, " ")
}

func (c ResourceCatalog) resourceType(s ResourceScope) (ResourceType, bool) {
	for _, t := range c.Types {
		if canonical(t.ClassCode) == s.ClassCode && canonical(t.FamilyCode) == s.FamilyCode && canonical(t.Code) == s.TypeCode {
			return t, true
		}
	}
	return ResourceType{}, false
}

func (c ResourceCatalog) presentationFields(s ResourceScope) []PresentationField {
	var fields []PresentationField
	for _, field := range c.PresentationFields {
		if field.Active && canonical(field.ClassCode) == s.ClassCode && canonical(field.FamilyCode) == s.FamilyCode && canonical(field.TypeCode) == s.TypeCode {
			fields = append(fields, field)
		}
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i].Position < fields[j].Position })
	return fields
}

func (c ResourceCatalog) definitionFor(code string) AttributeDefinition {
	code = canonicalAttribute(code)
	for _, definition := range c.Definitions {
		if canonicalAttribute(definition.Code) == code {
			return definition
		}
	}
	return AttributeDefinition{}
}
