package domain

import (
	"fmt"
	"strings"
)

// ResolutionRevision is the optimistic-concurrency revision of one line's
// explicit resolution decision history.
type ResolutionRevision uint64

// ResolutionOverrideAuditEntry is one append-only manual override decision.
type ResolutionOverrideAuditEntry struct {
	PurchaseLineID   int64
	PreviousOverride LinkStatus
	NewOverride      LinkStatus
	PreviousRevision ResolutionRevision
	NewRevision      ResolutionRevision
	Decision         MappingDecisionMetadata
}

// ResolutionOverrideTransition reports whether a valid decision changed the
// stored override and therefore requires persistence plus audit.
type ResolutionOverrideTransition struct {
	Changed bool
	Audit   *ResolutionOverrideAuditEntry
}

// SetResolutionOverrideCommand applies one explicit stored override. Derived
// states are intentionally not valid command values.
type SetResolutionOverrideCommand struct {
	LineID           int64
	Override         LinkStatus
	ExpectedRevision ResolutionRevision
	Actor            string
	Reason           string
	Decision         MappingDecisionMetadata
}

// EffectiveLineStatus derives the line status without mutating either the
// line or mapping. Explicit line overrides have precedence over aggregate
// knowledge, followed by identity conflict, resource inactivity, unresolved
// mapping, and confirmed active mapping.
func EffectiveLineStatus(override LinkStatus, mapping SupplierProductMapping, resourceActive bool) (LinkStatus, MappingCause) {
	switch override {
	case LinkNotApplicable:
		return LinkNotApplicable, MappingCauseLineNotApplicable
	case LinkConflict:
		return LinkConflict, MappingCauseLineConflictOverride
	}

	switch mapping.State(resourceActive) {
	case MappingStateIdentityConflict:
		return LinkConflict, MappingCauseIdentityConflict
	case MappingStateSuspended:
		return LinkSuspended, MappingCauseResourceInactive
	case MappingStateUnresolved:
		return LinkPending, MappingCauseUnresolved
	default:
		return LinkLinked, MappingCauseNone
	}
}

// EffectiveStatus derives this line's current status from its override and a
// current SupplierProduct mapping snapshot.
func (line PurchaseLine) EffectiveStatus(mapping SupplierProductMapping, resourceActive bool) (LinkStatus, MappingCause) {
	return EffectiveLineStatus(line.ResolutionOverride, mapping, resourceActive)
}

// ChangeResolutionOverride validates one manual decision and produces the
// append-only audit fact required when the stored override changes.
func (line *PurchaseLine) ChangeResolutionOverride(override LinkStatus, expected ResolutionRevision, decision MappingDecisionMetadata) (ResolutionOverrideTransition, error) {
	if !override.ValidOverride() {
		return ResolutionOverrideTransition{}, NewValidationError("resolution_override", "must be NONE, NO_APLICA, or CONFLICTO")
	}
	if strings.TrimSpace(decision.Actor) == "" || strings.TrimSpace(decision.Reason) == "" || decision.Origin != MappingOriginManual || decision.At.IsZero() {
		return ResolutionOverrideTransition{}, ErrInvalidDecisionMetadata
	}
	if line.ResolutionRevision != expected {
		return ResolutionOverrideTransition{}, fmt.Errorf("%w: expected %d, actual %d", ErrStaleResolutionRevision, expected, line.ResolutionRevision)
	}
	if line.ResolutionOverride == override {
		return ResolutionOverrideTransition{}, nil
	}
	entry := ResolutionOverrideAuditEntry{
		PurchaseLineID: line.ID, PreviousOverride: line.ResolutionOverride, NewOverride: override,
		PreviousRevision: line.ResolutionRevision, NewRevision: line.ResolutionRevision + 1, Decision: decision,
	}
	line.ResolutionOverride = override
	line.ResolutionRevision++
	return ResolutionOverrideTransition{Changed: true, Audit: &entry}, nil
}
