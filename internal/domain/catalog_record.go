package domain

import "context"

// CatalogRef identifies another catalog record by its own natural code (or
// "|"-joined composite identity, e.g. a Familia's "class|code") rather than
// a database ID. ApplyCatalogMutation (catalog_mutation.go) operates on the
// pure Go ResourceCatalog snapshot, which carries no ID concept at all
// (design D10) — every cross-reference inside that snapshot already resolves
// by the same natural keys (ClassCode, FamilyCode, ...), so CatalogRef
// follows the same convention rather than inventing a second identity
// system the pure domain layer cannot resolve without a repository.
type CatalogRef struct {
	Kind CatalogKindCode
	Code string
	// Label is the human-readable identity resolved by the repository for UI
	// presentation. Code remains the stable mutation/reference key.
	Label string
	// ID is the referenced record's own database identity, resolved by the
	// repository for public API addressing (so a client can PUT/deactivate/
	// reactivate the referenced record without a separate lookup) — the same
	// repository-resolved-presentation-data pattern as Label. Code remains
	// the stable mutation/reference key; ID is 0 when unresolved (e.g. a
	// pure in-memory ResourceCatalog snapshot with no repository behind it).
	ID int64
}

// CatalogValue is one field's value inside a CatalogRecord (design D8) — a
// typed union keyed by the owning FieldDescriptor.Kind, avoiding both the
// type-safety loss of map[string]any and the ~10x code blowup of one Go
// struct per kind.
type CatalogValue struct {
	Text string
	Bool bool
	Int  int
	List []string
	Ref  CatalogRef
}

// CatalogRecord is the generic transport shape for one row of any
// administrable CatalogKind (design D8). ID is the repository-assigned
// database identity (0 until Insert persists it) — the pure domain layer
// never uses it for cross-references, only the repository does.
// CatalogRuleRecord is one ordered applicability rule attached to a
// CatalogRecord. When is evaluated in Rules slice order; the remaining fields
// describe the rule's effective attribute semantics and lifecycle state.
type CatalogRuleRecord struct {
	When                 AttributeCondition
	Mode                 AttributeMode
	IdentityParticipates bool
	NotApplicable        bool
	Active               bool
}

// Revision is the migration-8 monotonic optimistic-concurrency (CAS) column
// for the record's underlying table (design "Resource revisions and
// identity"). It is 0 for a record not yet persisted; a repository populates
// it on Get/List/Insert. No current write path increments or checks it yet —
// dormant until a later CAS-aware slice wires it into Update/SetActive/Delete.
type CatalogRecord struct {
	Kind     CatalogKindCode
	ID       int64
	Revision uint64
	Active   bool
	Values   map[string]CatalogValue
	Rules    []CatalogRuleRecord
}

// cloneCatalogStrings preserves nil versus non-nil empty slices while
// isolating the mutable backing array.
func cloneCatalogStrings(values []string) []string {
	if values == nil {
		return nil
	}
	copyOfValues := make([]string, len(values))
	copy(copyOfValues, values)
	return copyOfValues
}

// cloneCatalogValue copies every mutable part of a catalog value. Text,
// scalar fields, and CatalogRef are value types; List is the nested storage
// that needs an independent backing array.
func cloneCatalogValue(value CatalogValue) CatalogValue {
	value.List = cloneCatalogStrings(value.List)
	return value
}

// cloneCatalogRules preserves nil versus non-nil empty slices while isolating
// the ordered rule records from their caller-owned backing array.
func cloneCatalogRules(rules []CatalogRuleRecord) []CatalogRuleRecord {
	if rules == nil {
		return nil
	}
	copyOfRules := make([]CatalogRuleRecord, len(rules))
	copy(copyOfRules, rules)
	return copyOfRules
}

// cloneCatalogRecord copies the values map and each value's nested storage so
// mutation builders can safely read a caller-owned record.
func cloneCatalogRecord(record CatalogRecord) CatalogRecord {
	copyOfRecord := record
	copyOfRecord.Rules = cloneCatalogRules(record.Rules)
	if record.Values == nil {
		return copyOfRecord
	}
	copyOfRecord.Values = make(map[string]CatalogValue, len(record.Values))
	for name, value := range record.Values {
		copyOfRecord.Values[name] = cloneCatalogValue(value)
	}
	return copyOfRecord
}

type CatalogStatus uint8

const (
	CatalogStatusActive CatalogStatus = iota
	CatalogStatusInactive
	CatalogStatusAll
)

// CatalogFilter narrows a CatalogAdminRepository.List call: Text is a
// case-insensitive partial match against the kind's Searchable fields,
// Parent narrows by one or more of the kind's own scoping fields (e.g.
// {"class": ...} when listing Familias). Status defaults and fails closed to
// Active so ordinary references cannot expose inactive records.
type CatalogFilter struct {
	Text   string
	Parent map[string]CatalogValue
	Status CatalogStatus
	Limit  int
	Offset int
}

// CatalogDependency reports how many rows of Kind reference one record,
// and whether that reference Blocks a guarded Desactivar/Delete (design §6).
type CatalogDependency struct {
	Kind     CatalogKindCode
	Count    int
	Blocking bool
}

// CatalogAdminRepository is the port every catalog-structure write goes
// through (design §3) — implemented in internal/postgres by a later PR
// (explicitly out of this PR's scope), consumed by internal/app/catalogo.
type CatalogAdminRepository interface {
	List(ctx context.Context, kind CatalogKindCode, f CatalogFilter) ([]CatalogRecord, error)
	Get(ctx context.Context, kind CatalogKindCode, id int64) (CatalogRecord, error)
	Insert(ctx context.Context, rec CatalogRecord) (int64, error)
	Update(ctx context.Context, rec CatalogRecord) error
	SetActive(ctx context.Context, kind CatalogKindCode, id int64, active bool) error
	Delete(ctx context.Context, kind CatalogKindCode, id int64) error
	Dependents(ctx context.Context, kind CatalogKindCode, id int64) ([]CatalogDependency, error)
	ReferencedByResources(ctx context.Context, kind CatalogKindCode, id int64) (bool, error)
}
