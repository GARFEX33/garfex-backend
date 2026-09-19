package domain

// LinkStatus is the functional linking state of one PurchaseLine toward a
// SupplierProduct and, through it, a Resource Master entry.
type LinkStatus string

const (
	// LinkPending means the line has no SupplierProduct, or its
	// SupplierProduct has no Resource Master relation yet.
	LinkPending LinkStatus = "PENDIENTE"
	// LinkLinked means the line's SupplierProduct is related to a Resource.
	LinkLinked LinkStatus = "VINCULADO"
	// LinkNotApplicable is set manually for a line that will never map to a
	// Resource Master entry (freight, taxes billed as a concept, and so on).
	LinkNotApplicable LinkStatus = "NO_APLICA"
	// LinkConflict is set manually when an automatic match would be unsafe
	// and a human must resolve the ambiguity.
	LinkConflict LinkStatus = "CONFLICTO"
)

// Valid reports whether s is one of the defined LinkStatus values.
func (s LinkStatus) Valid() bool {
	switch s {
	case LinkPending, LinkLinked, LinkNotApplicable, LinkConflict:
		return true
	default:
		return false
	}
}
