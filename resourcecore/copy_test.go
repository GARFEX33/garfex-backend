package resourcecore

import "testing"

func TestDefensiveCopy(t *testing.T) {
	orig := []string{"a", "b", "c"}
	cp := CloneStringSlice(orig)
	cp[0] = "z"
	if orig[0] != "a" || CloneStringSlice(nil) != nil || len(CloneStringSlice([]string{})) != 0 {
		t.Fatalf("string slice clone leaked")
	}
	v := Value{Kind: ValueStringList, Strings: []string{"x", "y"}}
	vcp := CloneValue(v)
	vcp.Strings[0] = "z"
	if v.Strings[0] != "x" {
		t.Fatalf("value clone leaked strings")
	}
	r := Value{Kind: ValueReference, Reference: &Reference{Kind: KindClass, ID: 1}}
	rcp := CloneValue(r)
	rcp.Reference.ID = 99
	if r.Reference.ID != 1 {
		t.Fatalf("value clone leaked reference")
	}
	rec := CatalogRecord{Kind: KindClass, Values: map[string]Value{"code": {Kind: ValueCode, Text: "C"}}, Rules: []ApplicabilityRule{{AttributeCode: "a", Equals: Value{Kind: ValueCode, Text: "V"}}}}
	reccp := CloneCatalogRecord(rec)
	reccp.Values["code"] = Value{Kind: ValueCode, Text: "X"}
	reccp.Rules[0].AttributeCode = "z"
	if rec.Values["code"].Text != "C" || rec.Rules[0].AttributeCode != "a" {
		t.Fatalf("record clone leaked")
	}
	desc := CatalogDescriptor{Kind: KindClass, Fields: []FieldDescriptor{{Name: "code", AllowCreate: true, EnumValues: []EnumValue{{Value: "A"}}}}, IdentityFields: []string{"code"}}
	descp := CloneCatalogDescriptor(desc)
	descp.Fields[0].EnumValues[0].Value = "B"
	descp.IdentityFields[0] = "x"
	if desc.Fields[0].EnumValues[0].Value != "A" || desc.IdentityFields[0] != "code" {
		t.Fatalf("descriptor clone leaked")
	}
	if !descp.Fields[0].AllowCreate {
		t.Fatalf("descriptor clone dropped AllowCreate")
	}
	res := Resource{ID: 1, Attributes: []AttributeValue{{Code: "a", Value: Value{Kind: ValueInteger, Text: "1"}}}}
	rescp := CloneResource(res)
	rescp.Attributes[0].Value.Text = "99"
	if res.Attributes[0].Value.Text != "1" {
		t.Fatalf("resource clone leaked")
	}

	keys := []AttributeOrderKey{{SourceLevel: "TYPE", SourceCode: "CABLE", CharacteristicCode: "color"}}
	keyscp := CloneAttributeOrderKeySlice(keys)
	keyscp[0].CharacteristicCode = "z"
	if keys[0].CharacteristicCode != "color" || CloneAttributeOrderKeySlice(nil) != nil {
		t.Fatalf("attribute order key slice clone leaked")
	}

	order := ResourceAttributeOrder{
		Scope:             ResourceScope{ClassCode: "MAT", FamilyCode: "CONDUCTORES", TypeCode: "CABLE"},
		OrderedAttributes: []AttributeOrderKey{{SourceLevel: "TYPE", SourceCode: "CABLE", CharacteristicCode: "color"}},
		OrderRevision:     "v1:abc",
	}
	ordercp := CloneResourceAttributeOrder(order)
	ordercp.OrderedAttributes[0].CharacteristicCode = "z"
	if order.OrderedAttributes[0].CharacteristicCode != "color" {
		t.Fatalf("resource attribute order clone leaked")
	}

	wreq := AttributeOrderWriteRequest{
		Actor:                 "PI",
		Scope:                 ResourceScope{ClassCode: "MAT", FamilyCode: "CONDUCTORES", TypeCode: "CABLE"},
		ExpectedOrderRevision: "v1:abc",
		OrderedAttributes:     []AttributeOrderKey{{SourceLevel: "TYPE", SourceCode: "CABLE", CharacteristicCode: "color"}},
	}
	wreqcp := CloneAttributeOrderWriteRequest(wreq)
	wreqcp.OrderedAttributes[0].CharacteristicCode = "z"
	if wreq.OrderedAttributes[0].CharacteristicCode != "color" {
		t.Fatalf("attribute order write request clone leaked")
	}
}
