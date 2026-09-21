package domain

import (
	"errors"
	"testing"
	"time"
)

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
	decision := MappingDecisionMetadata{Actor: "operator", Origin: MappingOriginManual, Reason: "reviewed", At: time.Now()}
	line := PurchaseLine{ID: 7, ResolutionOverride: LinkStatusNone}
	transition, err := line.ChangeResolutionOverride(LinkNotApplicable, 0, decision)
	if err != nil {
		t.Fatalf("ChangeResolutionOverride() error = %v", err)
	}
	if !transition.Changed || transition.Audit == nil || line.ResolutionOverride != LinkNotApplicable || line.ResolutionRevision != 1 {
		t.Fatalf("changed line/transition = %+v / %+v", line, transition)
	}
	if transition.Audit.PreviousOverride != LinkStatusNone || transition.Audit.NewOverride != LinkNotApplicable {
		t.Fatalf("audit = %+v", transition.Audit)
	}
	noOp, err := line.ChangeResolutionOverride(LinkNotApplicable, 1, decision)
	if err != nil || noOp.Changed {
		t.Fatalf("idempotent decision = %+v, %v", noOp, err)
	}
	if _, err := line.ChangeResolutionOverride(LinkConflict, 0, decision); !errors.Is(err, ErrStaleResolutionRevision) {
		t.Fatalf("stale error = %v", err)
	}
	if _, err := line.ChangeResolutionOverride(LinkPending, 1, decision); err == nil {
		t.Fatal("expected derived status to be rejected as an override")
	}
	if _, err := line.ChangeResolutionOverride(LinkStatusNone, 1, MappingDecisionMetadata{}); !errors.Is(err, ErrInvalidDecisionMetadata) {
		t.Fatalf("invalid decision error = %v", err)
	}
	if _, err := line.ChangeResolutionOverride(LinkStatusNone, 1, decision); err != nil {
		t.Fatalf("clear override error = %v", err)
	}
	if line.ResolutionOverride != LinkStatusNone || line.ResolutionRevision != 2 {
		t.Fatalf("cleared line = %+v", line)
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
