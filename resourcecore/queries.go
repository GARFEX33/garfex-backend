package resourcecore

// CatalogQuery narrows a catalog list. ClassCode, FamilyCode, TypeCode, and
// OptionSetCode scope the result to records whose parent reference matches
// that code; each is meaningful only for kinds whose descriptor names that
// parent field (e.g. FAMILIA by class; TIPO by class/family; APLICABILIDAD
// and PRESENTACION by class/family/type; OPCION by option set) and is
// silently ignored by any kind that does not have that parent — never an
// error.
type CatalogQuery struct {
	Kind                                           KindCode
	Scope                                          LifecycleScope
	Text                                           string
	ClassCode, FamilyCode, TypeCode, OptionSetCode string
	Limit, Offset                                  int
}
type ResourceQuery struct {
	Scope                                 LifecycleScope
	Text, ClassCode, FamilyCode, TypeCode string
	Limit, Offset                         int
}
type CatalogPage struct {
	Query                CatalogQuery
	Records              []CatalogRecord
	HasPrevious, HasNext bool
}
type ResourcePage struct {
	Query                ResourceQuery
	Resources            []Resource
	HasPrevious, HasNext bool
}
