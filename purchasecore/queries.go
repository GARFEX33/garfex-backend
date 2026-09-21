package purchasecore

import "time"

// ListCriteria paginates a history or list query.
type ListCriteria struct {
	Limit  int
	Offset int
}

// PurchaseLineQuery filters the global PurchaseLine workbench. A nil
// SupplierID means all suppliers; an empty EffectiveStatus means all derived
// statuses. Limit accepts 1..50; zero selects the default 50.
type PurchaseLineQuery struct {
	Limit           int
	Offset          int
	SupplierID      *int64
	EffectiveStatus LinkStatus
	DateFrom        *time.Time
	DateTo          *time.Time
	InvoiceText     string
	SupplierSKU     string
	Description     string
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

// PurchaseLinePage is one page of the global PurchaseLine workbench.
type PurchaseLinePage struct {
	Query       PurchaseLineQuery
	Rows        []PurchaseLineRow
	HasPrevious bool
	HasNext     bool
}
