package domain

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

// AttributeOrderKey identifies one effective occurrence within an exact scope.
// The same characteristic may occur at both FAMILY and TYPE levels.
type AttributeOrderKey struct {
	SourceLevel        string
	SourceCode         string
	CharacteristicCode string
}

// AttributeOrderMember associates a key with its internal binding incarnation.
// Persistence identifiers must never be mapped into the public ordered keys.
type AttributeOrderMember struct {
	Key                 AttributeOrderKey
	ResourceAttributeID int64
	Revision            uint64
}

// AttributeOrderState is authoritative input, not a client command. Members
// must contain the complete EffectiveAttributesFor baseline, in its order.
// SavedBindingIDs are persisted preferences; deleted bindings can be absent.
type AttributeOrderState struct {
	TypeID          int64
	HeadRevision    uint64
	Members         []AttributeOrderMember
	SavedBindingIDs []int64
}

// AttributeOrderSnapshot carries no persistence identifiers. OrderRevision is
// opaque and is a concurrency precondition, not an authorization credential.
type AttributeOrderSnapshot struct {
	Scope             ResourceScope
	OrderedAttributes []AttributeOrderKey
	OrderRevision     string
}

// ResolveAttributeOrder overlays saved bindings without changing presentation.
// The caller must resolve scope existence and load state coherently; this pure
// function validates the supplied exact scope and membership consistency.
func ResolveAttributeOrder(scope ResourceScope, state AttributeOrderState) (AttributeOrderSnapshot, error) {
	var err error
	scope, err = CanonicalAttributeOrderScope(scope)
	if err != nil {
		return AttributeOrderSnapshot{}, err
	}
	if state.TypeID <= 0 {
		return AttributeOrderSnapshot{}, validation("attribute order requires a resolved type incarnation")
	}
	members := make(map[int64]AttributeOrderMember, len(state.Members))
	keys := make(map[AttributeOrderKey]bool, len(state.Members))
	for _, member := range state.Members {
		key, err := member.Key.CanonicalFor(scope)
		if err != nil {
			return AttributeOrderSnapshot{}, err
		}
		if member.ResourceAttributeID <= 0 || keys[key] {
			return AttributeOrderSnapshot{}, validation("invalid or duplicate attribute order member")
		}
		if _, exists := members[member.ResourceAttributeID]; exists {
			return AttributeOrderSnapshot{}, validation("duplicate attribute order binding")
		}
		member.Key = key
		members[member.ResourceAttributeID], keys[key] = member, true
	}
	ordered := make([]AttributeOrderMember, 0, len(members))
	appendMember := func(id int64) {
		if member, exists := members[id]; exists {
			ordered = append(ordered, member)
			delete(members, id)
		}
	}
	for _, id := range state.SavedBindingIDs {
		appendMember(id)
	}
	for _, member := range state.Members {
		appendMember(member.ResourceAttributeID)
	}
	snapshot := AttributeOrderSnapshot{Scope: scope, OrderedAttributes: make([]AttributeOrderKey, len(ordered))}
	for i, member := range ordered {
		snapshot.OrderedAttributes[i] = member.Key
	}
	// A typed JSON object makes field boundaries unambiguous; the resolved order
	// contains the full membership and its incarnations, not just display codes.
	encoded, err := json.Marshal(struct {
		Scope        ResourceScope
		TypeID       int64
		HeadRevision uint64
		Members      []AttributeOrderMember
	}{Scope: scope, TypeID: state.TypeID, HeadRevision: state.HeadRevision, Members: ordered})
	if err != nil {
		return AttributeOrderSnapshot{}, fmt.Errorf("encode attribute order revision: %w", err)
	}
	snapshot.OrderRevision = fmt.Sprintf("v1:%x", sha256.Sum256(encoded))
	return snapshot, nil
}

// ValidatePermutation returns a canonical, detached full permutation.
func (s AttributeOrderSnapshot) ValidatePermutation(keys []AttributeOrderKey) ([]AttributeOrderKey, error) {
	if len(keys) != len(s.OrderedAttributes) {
		return nil, validation("attribute order must contain every effective occurrence")
	}
	remaining := make(map[AttributeOrderKey]bool, len(s.OrderedAttributes))
	for _, key := range s.OrderedAttributes {
		remaining[key] = true
	}
	result := make([]AttributeOrderKey, len(keys))
	for i, key := range keys {
		canonicalKey, err := key.CanonicalFor(s.Scope)
		if err != nil {
			return nil, err
		}
		if !remaining[canonicalKey] {
			return nil, validation("unknown or repeated attribute order key")
		}
		delete(remaining, canonicalKey)
		result[i] = canonicalKey
	}
	return result, nil
}

// CanonicalFor validates k belongs to scope and returns k with every field
// canonicalized (source level/code uppercase-normalized, characteristic code
// lowercase-normalized via canonicalAttribute) — the exact same
// canonicalization ResolveAttributeOrder and ValidatePermutation already
// apply internally to every key before comparing or indexing by it. A
// caller resolving a raw, DB-sourced key back to a persisted incarnation
// (e.g. a CAS writer mapping accepted keys to ResourceAttributeIDs) must
// canonicalize through this exact function, not reimplement it, since
// characteristic codes are stored as given (only comparisons canonicalize
// them — see resource_canonical.go's canonicalAttribute callers) and a
// mixed-case code otherwise silently fails to match.
func (k AttributeOrderKey) CanonicalFor(scope ResourceScope) (AttributeOrderKey, error) {
	k.SourceLevel = canonical(k.SourceLevel)
	k.SourceCode = canonical(k.SourceCode)
	k.CharacteristicCode = canonicalAttribute(k.CharacteristicCode)
	validFamily := k.SourceLevel == SourceLevelFamily && k.SourceCode == scope.FamilyCode
	validType := k.SourceLevel == SourceLevelType && k.SourceCode == scope.TypeCode
	if !validFamily && !validType {
		return AttributeOrderKey{}, validation("attribute order source does not belong to target scope")
	}
	if k.CharacteristicCode == "" {
		return AttributeOrderKey{}, validation("attribute order characteristic is required")
	}
	return k, nil
}
