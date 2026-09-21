package purchasecore

// ListCriteria paginates a history or list query.
type ListCriteria struct {
	Limit  int
	Offset int
}

// PurchasePage is one page of a supplier's purchase history, most recent
// purchase first.
type PurchasePage struct {
	Query       ListCriteria
	Purchases   []Purchase
	HasPrevious bool
	HasNext     bool
}

// SupplierProductPage is one page of a supplier's registered products.
type SupplierProductPage struct {
	Query       ListCriteria
	Products    []SupplierProduct
	HasPrevious bool
	HasNext     bool
}

// PurchaseLineHistoryPage is one page of a Resource Master entry's
// purchase history, most recent purchase first.
type PurchaseLineHistoryPage struct {
	Query       ListCriteria
	History     []PurchaseLineHistory
	HasPrevious bool
	HasNext     bool
}

type MappingAuditPage struct {
	Query       ListCriteria
	Entries     []MappingAuditEntry
	HasPrevious bool
	HasNext     bool
}
