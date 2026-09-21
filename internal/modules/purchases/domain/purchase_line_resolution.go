package domain

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

// SetResolutionOverride stores only a permitted line-specific override. The
// legacy LinkStatus field is updated as a compatibility projection for this
// explicit override; derived statuses must come from EffectiveStatus.
func (line PurchaseLine) SetResolutionOverride(override LinkStatus) (PurchaseLine, error) {
	if !override.ValidOverride() {
		return PurchaseLine{}, NewValidationError("resolution_override", "must be NONE, NO_APLICA, or CONFLICTO")
	}
	line.ResolutionOverride = override
	line.LinkStatus = override
	return line, nil
}

// ClearResolutionOverride removes the line-specific override.
func (line PurchaseLine) ClearResolutionOverride() PurchaseLine {
	line.ResolutionOverride = LinkStatusNone
	line.LinkStatus = LinkStatusNone
	return line
}
