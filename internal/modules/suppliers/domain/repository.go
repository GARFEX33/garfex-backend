package domain

import "context"

type SupplierSearch struct {
	Text   string
	Active *bool
	Limit  int
	Offset int
}

type ListCriteria struct {
	// Text is matched case-insensitively against the branch's searchable text fields.
	Text   string
	Active *bool
	Limit  int
	Offset int
}

type ContactListCriteria struct {
	// Text is matched case-insensitively against the contact's searchable text fields.
	Text     string
	Active   *bool
	BranchID *int64
	Limit    int
	Offset   int
}

// Repository is the persistence port for the complete Supplier Master core.
// Every child read and write is explicitly scoped by SupplierID.
type Repository interface {
	CreateSupplier(context.Context, Supplier) (Supplier, error)
	GetSupplier(context.Context, int64) (Supplier, error)
	// GetSupplierByTaxIdentifier finds one supplier by its tax identifier,
	// compared exactly and case/space-insensitively like the unique index,
	// regardless of active state. taxID arrives already normalized.
	GetSupplierByTaxIdentifier(context.Context, string) (Supplier, error)
	SearchSuppliers(context.Context, SupplierSearch) ([]Supplier, error)
	UpdateSupplier(context.Context, Supplier) (Supplier, error)
	SetSupplierActive(context.Context, int64, bool) (Supplier, error)

	CreateBranch(context.Context, Branch) (Branch, error)
	GetBranch(context.Context, int64, int64) (Branch, error)
	ListBranches(context.Context, int64, ListCriteria) ([]Branch, error)
	UpdateBranch(context.Context, Branch) (Branch, error)
	SetBranchActive(context.Context, int64, int64, bool) (Branch, error)

	CreateContact(context.Context, Contact) (Contact, error)
	GetContact(context.Context, int64, int64) (Contact, error)
	ListContacts(context.Context, int64, ContactListCriteria) ([]Contact, error)
	UpdateContact(context.Context, Contact) (Contact, error)
	SetContactActive(context.Context, int64, int64, bool) (Contact, error)
}
