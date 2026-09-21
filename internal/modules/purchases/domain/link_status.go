package domain

// LinkStatus is the effective or persisted override status of one
// PurchaseLine. Only LinkStatusNone, LinkNotApplicable, and LinkConflict are
// valid persisted line-resolution overrides. Pending, linked, and suspended
// are derived from current mapping knowledge and Resource.Active.
type LinkStatus string

const (
	// LinkStatusNone means the line has no resolution override.
	LinkStatusNone LinkStatus = "NONE"
	// LinkPending means mapping knowledge is unresolved.
	LinkPending LinkStatus = "PENDIENTE"
	// LinkLinked means a confirmed mapping points to an active resource.
	LinkLinked LinkStatus = "VINCULADO"
	// LinkSuspended means the confirmed target resource is inactive.
	LinkSuspended LinkStatus = "SUSPENDIDO"
	// LinkNotApplicable is a line-specific override for lines that will never
	// map to a Resource Master entry.
	LinkNotApplicable LinkStatus = "NO_APLICA"
	// LinkConflict is a line-specific conflict override, or the derived
	// projection of SupplierProduct identity conflict.
	LinkConflict LinkStatus = "CONFLICTO"
)

// Valid reports whether s is a defined effective status.
func (s LinkStatus) Valid() bool {
	switch s {
	case LinkPending, LinkLinked, LinkSuspended, LinkNotApplicable, LinkConflict:
		return true
	default:
		return false
	}
}

// ValidOverride reports whether s is allowed as persisted line authority.
func (s LinkStatus) ValidOverride() bool {
	return s == LinkStatusNone || s == LinkNotApplicable || s == LinkConflict
}
