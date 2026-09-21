package domain

import (
	"fmt"
	"strings"
	"time"
)

// MappingRevision is the optimistic-concurrency revision of confirmed
// SupplierProduct mapping knowledge.
type MappingRevision uint64

// MappingState is the deterministic state of mapping knowledge. Suspended is
// derived from Resource.Active only when a ResourceID exists; it is never
// stored in SupplierProductMapping.
type MappingState string

const (
	MappingStateUnresolved       MappingState = "UNRESOLVED"
	MappingStateConfirmed        MappingState = "CONFIRMED"
	MappingStateSuspended        MappingState = "SUSPENDED"
	MappingStateIdentityConflict MappingState = "IDENTITY_CONFLICT"
)

// MappingCause explains the state selected by the mapping and line-resolution
// precedence rules.
type MappingCause string

const (
	MappingCauseNone                 MappingCause = "NONE"
	MappingCauseUnresolved           MappingCause = "UNRESOLVED"
	MappingCauseResourceInactive     MappingCause = "RESOURCE_INACTIVE"
	MappingCauseIdentityConflict     MappingCause = "IDENTITY_CONFLICT"
	MappingCauseLineNotApplicable    MappingCause = "LINE_NOT_APPLICABLE"
	MappingCauseLineConflictOverride MappingCause = "LINE_CONFLICT_OVERRIDE"
)

// MappingOrigin identifies the currently supported source of a human mapping
// decision. Automated or provider origins are intentionally not modeled.
type MappingOrigin string

const MappingOriginManual MappingOrigin = "manual"

// MappingDecisionMetadata is required for every semantic mapping transition.
type MappingDecisionMetadata struct {
	Actor  string
	Origin MappingOrigin
	Reason string
	At     time.Time
}

func (m MappingDecisionMetadata) validate(requireReason bool) error {
	if strings.TrimSpace(m.Actor) == "" {
		return fmt.Errorf("%w: actor is required", ErrInvalidDecisionMetadata)
	}
	if m.Origin != MappingOriginManual {
		return fmt.Errorf("%w: unsupported origin", ErrInvalidDecisionMetadata)
	}
	if requireReason && strings.TrimSpace(m.Reason) == "" {
		return fmt.Errorf("%w: reason is required", ErrInvalidDecisionMetadata)
	}
	if m.At.IsZero() {
		return fmt.Errorf("%w: time is required", ErrInvalidDecisionMetadata)
	}
	return nil
}

// SupplierProductMapping is the value object holding confirmed mapping
// knowledge. Resource activity is deliberately not part of this value object.
type SupplierProductMapping struct {
	ResourceID       *int64
	IdentityConflict bool
}

// NewUnresolvedSupplierProductMapping returns an empty mapping knowledge
// value.
func NewUnresolvedSupplierProductMapping() SupplierProductMapping {
	return SupplierProductMapping{}
}

// NewConfirmedSupplierProductMapping returns mapping knowledge for resourceID.
// Invalid resource IDs produce the zero unresolved value; transition methods
// perform the authoritative validation and return an error.
func NewConfirmedSupplierProductMapping(resourceID int64) SupplierProductMapping {
	if resourceID <= 0 {
		return SupplierProductMapping{}
	}
	return SupplierProductMapping{ResourceID: int64Pointer(resourceID)}
}

// KnowledgeState derives the state without consulting Resource.Active.
func (m SupplierProductMapping) KnowledgeState() MappingState {
	if m.IdentityConflict {
		return MappingStateIdentityConflict
	}
	if m.ResourceID == nil {
		return MappingStateUnresolved
	}
	return MappingStateConfirmed
}

// State derives effective mapping state. Resource activity is an input and is
// never persisted as mapping authority.
func (m SupplierProductMapping) State(resourceActive bool) MappingState {
	if m.IdentityConflict {
		return MappingStateIdentityConflict
	}
	if m.ResourceID == nil {
		return MappingStateUnresolved
	}
	if !resourceActive {
		return MappingStateSuspended
	}
	return MappingStateConfirmed
}

// EffectiveMappingState derives the aggregate's current effective state from
// mapping knowledge and the current Resource.Active input.
func (sp SupplierProduct) EffectiveMappingState(resourceActive bool) MappingState {
	return sp.CurrentMapping.State(resourceActive)
}

// EffectiveMappingCause derives the aggregate's current effective cause.
func (sp SupplierProduct) EffectiveMappingCause(resourceActive bool) MappingCause {
	return sp.CurrentMapping.Cause(resourceActive)
}

// Cause derives the reason for the effective mapping state.
func (m SupplierProductMapping) Cause(resourceActive bool) MappingCause {
	switch m.State(resourceActive) {
	case MappingStateIdentityConflict:
		return MappingCauseIdentityConflict
	case MappingStateSuspended:
		return MappingCauseResourceInactive
	case MappingStateUnresolved:
		return MappingCauseUnresolved
	default:
		return MappingCauseNone
	}
}

func (m SupplierProductMapping) equal(other SupplierProductMapping) bool {
	if m.IdentityConflict != other.IdentityConflict {
		return false
	}
	if m.ResourceID == nil || other.ResourceID == nil {
		return m.ResourceID == nil && other.ResourceID == nil
	}
	return *m.ResourceID == *other.ResourceID
}

func (m SupplierProductMapping) copy() SupplierProductMapping {
	if m.ResourceID == nil {
		return m
	}
	resourceID := *m.ResourceID
	m.ResourceID = &resourceID
	return m
}

// MappingOperation identifies one of the five authorized knowledge
// transitions.
type MappingOperation string

const (
	MappingOperationConfirm                 MappingOperation = "CONFIRM"
	MappingOperationCorrect                 MappingOperation = "CORRECT"
	MappingOperationExceptionalUnlink       MappingOperation = "EXCEPTIONAL_UNLINK"
	MappingOperationReportIdentityConflict  MappingOperation = "REPORT_IDENTITY_CONFLICT"
	MappingOperationResolveIdentityConflict MappingOperation = "RESOLVE_IDENTITY_CONFLICT"
)

// MappingAuditEntry is an append-only record produced by a changed semantic
// mapping transition.
type MappingAuditEntry struct {
	SupplierProductID int64
	PreviousMapping   SupplierProductMapping
	NewMapping        SupplierProductMapping
	PreviousState     MappingState
	NewState          MappingState
	PreviousRevision  MappingRevision
	NewRevision       MappingRevision
	Operation         MappingOperation
	Decision          MappingDecisionMetadata
}

// MappingTransition reports whether an aggregate changed and, when it did,
// exposes the audit entry that must be appended by persistence.
type MappingTransition struct {
	Changed bool
	Audit   *MappingAuditEntry
}

func (sp *SupplierProduct) transition(operation MappingOperation, next SupplierProductMapping, expected MappingRevision, decision MappingDecisionMetadata) (MappingTransition, error) {
	if err := decision.validate(operation != MappingOperationConfirm); err != nil {
		return MappingTransition{}, err
	}
	if sp.MappingRevision != expected {
		return MappingTransition{}, fmt.Errorf("%w: expected %d, actual %d", ErrStaleMappingRevision, expected, sp.MappingRevision)
	}
	if next.ResourceID != nil && *next.ResourceID <= 0 {
		return MappingTransition{}, NewValidationError("resource_id", "must be positive")
	}

	previous := sp.CurrentMapping.copy()
	entry := MappingAuditEntry{
		SupplierProductID: sp.ID,
		PreviousMapping:   previous,
		NewMapping:        next.copy(),
		PreviousState:     previous.KnowledgeState(),
		NewState:          next.KnowledgeState(),
		PreviousRevision:  sp.MappingRevision,
		NewRevision:       sp.MappingRevision + 1,
		Operation:         operation,
		Decision:          decision,
	}
	sp.CurrentMapping = next.copy()
	sp.MappingRevision++
	audit := entry
	return MappingTransition{Changed: true, Audit: &audit}, nil
}

func int64Pointer(value int64) *int64 { return &value }
