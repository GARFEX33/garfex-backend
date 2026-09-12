package resourcecore

// CatalogQuery narrows a catalog list. ClassCode and FamilyCode scope the
// result to records whose parent reference matches that code; they are
// meaningful only for kinds whose descriptor names that parent field (e.g.
// FAMILIA by class, TIPO by class/family) and are silently ignored by any
// kind that does not have that parent — never an error.
type CatalogQuery struct {
	Kind                  KindCode
	Scope                 LifecycleScope
	Text                  string
	ClassCode, FamilyCode string
	Limit, Offset         int
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
