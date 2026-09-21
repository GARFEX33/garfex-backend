package domain

import (
	"errors"
	"testing"
	"time"
)

func validMappingDecision() MappingDecisionMetadata {
	return MappingDecisionMetadata{
		Actor:  "operator-1",
		Origin: MappingOriginManual,
		Reason: "verified against supplier documentation",
		At:     time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
}

func newUnresolvedSupplierProduct() SupplierProduct {
	return SupplierProduct{
		ID:             7,
		SupplierID:     11,
		SupplierSKU:    "SKU-1",
		CurrentMapping: NewUnresolvedSupplierProductMapping(),
	}
}

func TestSupplierProductMappingTransitions(t *testing.T) {
	sp := newUnresolvedSupplierProduct()
	decision := validMappingDecision()

	confirmed, err := sp.ConfirmMapping(42, 0, decision)
	if err != nil {
		t.Fatalf("ConfirmMapping() error = %v", err)
	}
	if !confirmed.Changed || confirmed.Audit == nil {
		t.Fatalf("ConfirmMapping() = %+v, want changed audit", confirmed)
	}
	if sp.MappingRevision != 1 || sp.CurrentMapping.ResourceID == nil || *sp.CurrentMapping.ResourceID != 42 {
		t.Fatalf("confirmed mapping = %+v revision %d", sp.CurrentMapping, sp.MappingRevision)
	}
	if confirmed.Audit.Operation != MappingOperationConfirm || confirmed.Audit.SupplierProductID != sp.ID {
		t.Fatalf("audit = %+v, want one confirm entry for the aggregate", confirmed.Audit)
	}
	if confirmed.Audit.Decision != decision {
		t.Fatalf("audit decision = %+v, want %+v", confirmed.Audit.Decision, decision)
	}

	// Confirming the same target is an idempotent no-op.
	reconfirmed, err := sp.ConfirmMapping(42, 1, decision)
	if err != nil {
		t.Fatalf("same-target ConfirmMapping() error = %v", err)
	}
	if reconfirmed.Changed || reconfirmed.Audit != nil || sp.MappingRevision != 1 {
		t.Fatalf("same-target confirmation = %+v, revision %d; want no change", reconfirmed, sp.MappingRevision)
	}

	corrected, err := sp.CorrectMapping(42, 43, 1, decision)
	if err != nil {
		t.Fatalf("CorrectMapping() error = %v", err)
	}
	if !corrected.Changed || sp.MappingRevision != 2 || *sp.CurrentMapping.ResourceID != 43 {
		t.Fatalf("corrected mapping = %+v revision %d", sp.CurrentMapping, sp.MappingRevision)
	}

	unlinked, err := sp.ExceptionalUnlink(43, 2, decision)
	if err != nil {
		t.Fatalf("ExceptionalUnlink() error = %v", err)
	}
	if !unlinked.Changed || sp.MappingRevision != 3 || sp.CurrentMapping.ResourceID != nil {
		t.Fatalf("unlinked mapping = %+v revision %d", sp.CurrentMapping, sp.MappingRevision)
	}
}

func TestSupplierProductMappingConflictTransitions(t *testing.T) {
	sp := newUnresolvedSupplierProduct()
	decision := validMappingDecision()
	if _, err := sp.ConfirmMapping(42, 0, decision); err != nil {
		t.Fatal(err)
	}

	reported, err := sp.ReportIdentityConflict(42, 1, decision)
	if err != nil {
		t.Fatalf("ReportIdentityConflict() error = %v", err)
	}
	if !reported.Changed || sp.MappingRevision != 2 || !sp.CurrentMapping.IdentityConflict {
		t.Fatalf("reported mapping = %+v revision %d", sp.CurrentMapping, sp.MappingRevision)
	}
	if got := sp.CurrentMapping.State(true); got != MappingStateIdentityConflict {
		t.Fatalf("state = %q, want identity conflict", got)
	}

	resolved, err := sp.ResolveIdentityConflict(42, 43, 2, decision)
	if err != nil {
		t.Fatalf("ResolveIdentityConflict() error = %v", err)
	}
	if !resolved.Changed || sp.MappingRevision != 3 || sp.CurrentMapping.IdentityConflict || *sp.CurrentMapping.ResourceID != 43 {
		t.Fatalf("resolved mapping = %+v revision %d", sp.CurrentMapping, sp.MappingRevision)
	}
}

func TestSupplierProductMappingRejectsInvalidMetadataStaleRevisionAndTransition(t *testing.T) {
	sp := newUnresolvedSupplierProduct()
	invalid := validMappingDecision()
	invalid.Actor = ""
	if _, err := sp.ConfirmMapping(42, 0, invalid); !errors.Is(err, ErrInvalidDecisionMetadata) {
		t.Fatalf("invalid metadata error = %v, want ErrInvalidDecisionMetadata", err)
	}

	if _, err := sp.ConfirmMapping(42, 1, validMappingDecision()); !errors.Is(err, ErrStaleMappingRevision) {
		t.Fatalf("stale revision error = %v, want ErrStaleMappingRevision", err)
	}
	if _, err := sp.CorrectMapping(42, 42, 0, validMappingDecision()); !errors.Is(err, ErrInvalidMappingTransition) {
		t.Fatalf("invalid transition error = %v, want ErrInvalidMappingTransition", err)
	}
}

func TestSupplierProductMappingRejectsEachInvalidTransition(t *testing.T) {
	decision := validMappingDecision()
	tests := []struct {
		name string
		call func(*SupplierProduct) error
	}{
		{name: "correct unresolved", call: func(sp *SupplierProduct) error { _, err := sp.CorrectMapping(42, 42, 0, decision); return err }},
		{name: "unlink unresolved", call: func(sp *SupplierProduct) error { _, err := sp.ExceptionalUnlink(42, 0, decision); return err }},
		{name: "report unresolved", call: func(sp *SupplierProduct) error { _, err := sp.ReportIdentityConflict(42, 0, decision); return err }},
		{name: "resolve unresolved", call: func(sp *SupplierProduct) error { _, err := sp.ResolveIdentityConflict(42, 42, 0, decision); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(&SupplierProduct{CurrentMapping: NewUnresolvedSupplierProductMapping()}); !errors.Is(err, ErrInvalidMappingTransition) {
				t.Fatalf("error = %v, want ErrInvalidMappingTransition", err)
			}
		})
	}

	sp := newUnresolvedSupplierProduct()
	if _, err := sp.ConfirmMapping(42, 0, decision); err != nil {
		t.Fatal(err)
	}
	if _, err := sp.ConfirmMapping(43, 1, decision); !errors.Is(err, ErrInvalidMappingTransition) {
		t.Fatalf("different confirm error = %v, want ErrInvalidMappingTransition", err)
	}
	if _, err := sp.CorrectMapping(42, 43, 1, decision); err != nil {
		t.Fatal(err)
	}
	if _, err := sp.CorrectMapping(43, 44, 1, decision); !errors.Is(err, ErrStaleMappingRevision) {
		t.Fatalf("stale correct error = %v, want ErrStaleMappingRevision", err)
	}
}

func TestSupplierProductMappingRejectsMismatchedExpectedResources(t *testing.T) {
	decision := validMappingDecision()
	sp := newUnresolvedSupplierProduct()
	if _, err := sp.ConfirmMapping(42, 0, decision); err != nil {
		t.Fatal(err)
	}

	if _, err := sp.CorrectMapping(99, 43, 1, decision); !errors.Is(err, ErrInvalidMappingTransition) {
		t.Fatalf("correct expected resource error = %v, want ErrInvalidMappingTransition", err)
	}
	if _, err := sp.ExceptionalUnlink(99, 1, decision); !errors.Is(err, ErrInvalidMappingTransition) {
		t.Fatalf("unlink expected resource error = %v, want ErrInvalidMappingTransition", err)
	}
	if _, err := sp.ReportIdentityConflict(99, 1, decision); !errors.Is(err, ErrInvalidMappingTransition) {
		t.Fatalf("report expected resource error = %v, want ErrInvalidMappingTransition", err)
	}

	if _, err := sp.ReportIdentityConflict(42, 1, decision); err != nil {
		t.Fatal(err)
	}
	if _, err := sp.ResolveIdentityConflict(99, 43, 2, decision); !errors.Is(err, ErrInvalidMappingTransition) {
		t.Fatalf("resolve expected resource error = %v, want ErrInvalidMappingTransition", err)
	}
}

func TestSupplierProductMappingDecisionReasons(t *testing.T) {
	decision := validMappingDecision()
	decision.Reason = ""
	sp := newUnresolvedSupplierProduct()
	if _, err := sp.ConfirmMapping(42, 0, decision); err != nil {
		t.Fatalf("blank confirm reason error = %v, want allowed", err)
	}
	if noOp, err := sp.ConfirmMapping(42, 1, decision); err != nil || noOp.Changed || sp.MappingRevision != 1 {
		t.Fatalf("blank same-target confirmation = %+v, %v; want idempotent no-op", noOp, err)
	}
	if _, err := sp.CorrectMapping(42, 43, 1, decision); !errors.Is(err, ErrInvalidDecisionMetadata) {
		t.Fatalf("blank correction reason error = %v, want ErrInvalidDecisionMetadata", err)
	}

	tests := []struct {
		name string
		call func(*SupplierProduct, MappingDecisionMetadata) error
	}{
		{name: "exceptional unlink", call: func(sp *SupplierProduct, d MappingDecisionMetadata) error {
			_, err := sp.ExceptionalUnlink(42, 1, d)
			return err
		}},
		{name: "report identity conflict", call: func(sp *SupplierProduct, d MappingDecisionMetadata) error {
			_, err := sp.ReportIdentityConflict(42, 1, d)
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			product := newUnresolvedSupplierProduct()
			if _, err := product.ConfirmMapping(42, 0, validMappingDecision()); err != nil {
				t.Fatal(err)
			}
			if err := tt.call(&product, decision); !errors.Is(err, ErrInvalidDecisionMetadata) {
				t.Fatalf("error = %v, want ErrInvalidDecisionMetadata", err)
			}
		})
	}

	conflicted := newUnresolvedSupplierProduct()
	if _, err := conflicted.ConfirmMapping(42, 0, validMappingDecision()); err != nil {
		t.Fatal(err)
	}
	if _, err := conflicted.ReportIdentityConflict(42, 1, validMappingDecision()); err != nil {
		t.Fatal(err)
	}
	if _, err := conflicted.ResolveIdentityConflict(42, 43, 2, decision); !errors.Is(err, ErrInvalidDecisionMetadata) {
		t.Fatalf("blank resolve reason error = %v, want ErrInvalidDecisionMetadata", err)
	}
}

func TestSupplierProductMappingDerivedStatePrecedence(t *testing.T) {
	mapping := NewConfirmedSupplierProductMapping(42)
	if got := mapping.State(false); got != MappingStateSuspended {
		t.Fatalf("inactive state = %q, want suspended", got)
	}
	mapping.IdentityConflict = true
	if got := mapping.State(false); got != MappingStateIdentityConflict {
		t.Fatalf("conflict state = %q, want identity conflict", got)
	}
	if got := NewUnresolvedSupplierProductMapping().State(false); got != MappingStateUnresolved {
		t.Fatalf("unresolved state = %q, want unresolved", got)
	}
}

func TestSupplierProductDescriptionDoesNotChangeMappingRevision(t *testing.T) {
	sp := newUnresolvedSupplierProduct()
	sp.MappingRevision = 4
	updated := sp.UpdateDescription("last seen description")
	if updated.Description != "last seen description" || updated.MappingRevision != 4 {
		t.Fatalf("updated product = %+v, want description-only update", updated)
	}
}
