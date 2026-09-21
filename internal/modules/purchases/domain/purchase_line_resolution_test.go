package domain

import "testing"

func TestEffectiveLineStatusPrecedence(t *testing.T) {
	mapping := NewConfirmedSupplierProductMapping(42)
	tests := []struct {
		name        string
		override    LinkStatus
		mapping     SupplierProductMapping
		resourceAct bool
		wantStatus  LinkStatus
		wantCause   MappingCause
	}{
		{name: "not applicable overrides identity conflict", override: LinkNotApplicable, mapping: SupplierProductMapping{ResourceID: int64Pointer(42), IdentityConflict: true}, resourceAct: false, wantStatus: LinkNotApplicable, wantCause: MappingCauseLineNotApplicable},
		{name: "line conflict overrides identity conflict", override: LinkConflict, mapping: SupplierProductMapping{ResourceID: int64Pointer(42), IdentityConflict: true}, resourceAct: false, wantStatus: LinkConflict, wantCause: MappingCauseLineConflictOverride},
		{name: "identity conflict outranks inactive", mapping: SupplierProductMapping{ResourceID: int64Pointer(42), IdentityConflict: true}, resourceAct: false, wantStatus: LinkConflict, wantCause: MappingCauseIdentityConflict},
		{name: "inactive resource is suspended", mapping: mapping, resourceAct: false, wantStatus: LinkSuspended, wantCause: MappingCauseResourceInactive},
		{name: "unresolved mapping stays pending when inactive input is supplied", mapping: NewUnresolvedSupplierProductMapping(), resourceAct: false, wantStatus: LinkPending, wantCause: MappingCauseUnresolved},
		{name: "unresolved mapping stays pending when active", mapping: NewUnresolvedSupplierProductMapping(), resourceAct: true, wantStatus: LinkPending, wantCause: MappingCauseUnresolved},
		{name: "confirmed active mapping is linked", mapping: mapping, resourceAct: true, wantStatus: LinkLinked, wantCause: MappingCauseNone},
		{name: "zero values are unresolved pending", mapping: SupplierProductMapping{}, resourceAct: false, wantStatus: LinkPending, wantCause: MappingCauseUnresolved},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, cause := EffectiveLineStatus(tt.override, tt.mapping, tt.resourceAct)
			if status != tt.wantStatus || cause != tt.wantCause {
				t.Fatalf("EffectiveLineStatus() = %q, %q; want %q, %q", status, cause, tt.wantStatus, tt.wantCause)
			}
		})
	}
}

func TestPurchaseLineResolutionOverride(t *testing.T) {
	line := PurchaseLine{}
	line, err := line.SetResolutionOverride(LinkNotApplicable)
	if err != nil {
		t.Fatalf("SetResolutionOverride() error = %v", err)
	}
	if line.ResolutionOverride != LinkNotApplicable {
		t.Fatalf("line = %+v, want not-applicable override", line)
	}
	if _, err := line.SetResolutionOverride(LinkPending); err == nil {
		t.Fatal("expected derived status to be rejected as an override")
	}
	line = line.ClearResolutionOverride()
	if line.ResolutionOverride != LinkStatusNone {
		t.Fatalf("cleared line = %+v, want no override", line)
	}
}

func TestLinkStatusValidOverride(t *testing.T) {
	for _, status := range []LinkStatus{LinkStatusNone, LinkNotApplicable, LinkConflict} {
		if !status.ValidOverride() {
			t.Fatalf("ValidOverride(%q) = false", status)
		}
	}
	for _, status := range []LinkStatus{LinkPending, LinkLinked, LinkSuspended} {
		if status.ValidOverride() {
			t.Fatalf("ValidOverride(%q) = true, want false", status)
		}
	}
}
