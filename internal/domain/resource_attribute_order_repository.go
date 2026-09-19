package domain

import (
	"context"
	"errors"
)

// AttributeOrderReader loads order and structural catalog from one committed
// snapshot, without publishing either into a process-local catalog authority.
type AttributeOrderReader interface {
	ReadAttributeOrder(context.Context, ResourceScope) (AttributeOrderReadResult, error)
}

// AttributeOrderWriteRequest is the CAS command for AttributeOrderWriter.
// ExpectedOrderRevision must equal the OrderRevision a caller most recently
// observed from AttributeOrderReader.ReadAttributeOrder (or a prior
// WriteAttributeOrder result) for the exact same scope; OrderedAttributes
// must be an exact permutation of that same snapshot's OrderedAttributes
// (see AttributeOrderSnapshot.ValidatePermutation) — the writer re-resolves
// both against a freshly locked read and never trusts the caller's copy.
type AttributeOrderWriteRequest struct {
	Scope                 ResourceScope
	ExpectedOrderRevision string
	OrderedAttributes     []AttributeOrderKey
}

// AttributeOrderWriter performs one locked, compare-and-swap reorder
// transaction: implementations must take locks before reading, re-derive the
// coherent structural baseline under lock, reject a stale
// ExpectedOrderRevision (ErrAttributeOrderRevisionConflict) or a malformed
// permutation (ErrResourceValidation), atomically replace the full persisted
// membership, and always bump the head revision for an accepted write —
// including a no-op reorder — so the same token has exactly one winner.
type AttributeOrderWriter interface {
	WriteAttributeOrder(context.Context, AttributeOrderWriteRequest) (AttributeOrderReadResult, error)
}

// AttributeOrderStore combines both attribute-order capabilities for a
// caller (or constructor) that needs read and CAS-write access from one
// value, without widening either narrower interface's own contract.
type AttributeOrderStore interface {
	AttributeOrderReader
	AttributeOrderWriter
}

// ErrAttributeOrderRevisionConflict classifies a WriteAttributeOrder call
// whose ExpectedOrderRevision no longer matches the freshly, lock-coherent
// resolved OrderRevision — distinct from ErrCatalogRecordNotFound (the scope
// itself does not exist) and ErrResourceValidation (a malformed permutation)
// — never inferred from an error string, mirroring
// ErrResourceRevisionConflict's disambiguation discipline
// (resource_repository_v2.go).
var ErrAttributeOrderRevisionConflict = errors.New("attribute order revision conflict")

// ErrAttributeOrderUnavailable classifies a WriteAttributeOrder call whose
// commit outcome is ambiguous — specifically, the underlying transaction's
// own Commit call returned an error, so whether the write actually applied
// cannot be determined from that error alone. It is never used for a
// deterministic pre-commit rejection (those return
// ErrAttributeOrderRevisionConflict, ErrResourceValidation, or
// ErrCatalogRecordNotFound instead, and the transaction is cleanly rolled
// back). A caller observing ErrAttributeOrderUnavailable MUST perform a
// fresh ReadAttributeOrder and decide from the newly observed revision; it
// must never blindly replay the same ExpectedOrderRevision/OrderedAttributes
// pair, since a successful-but-unacknowledged commit would then look like a
// stale write against a state that no longer exists.
var ErrAttributeOrderUnavailable = errors.New("attribute order commit outcome unavailable")

// AttributeOrderReadResult lets callers evaluate and order attributes against
// the same catalog version. Persistence incarnations stay behind this result.
type AttributeOrderReadResult struct {
	Catalog ResourceCatalog
	Order   AttributeOrderSnapshot
}

func NewAttributeOrderReadResult(catalog ResourceCatalog, order AttributeOrderSnapshot) AttributeOrderReadResult {
	order.OrderedAttributes = append([]AttributeOrderKey{}, order.OrderedAttributes...)
	return AttributeOrderReadResult{Catalog: cloneResourceCatalog(catalog), Order: order}
}

// CanonicalAttributeOrderScope checks exact-scope shape, not existence.
func CanonicalAttributeOrderScope(scope ResourceScope) (ResourceScope, error) {
	scope = scope.canonicalize()
	incomplete := scope.ClassCode == "" || scope.FamilyCode == "" || scope.TypeCode == ""
	if incomplete {
		return ResourceScope{}, validation("attribute order requires an exact class/family/type scope")
	}
	return scope, nil
}
