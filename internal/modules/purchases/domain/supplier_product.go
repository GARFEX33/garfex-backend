package domain

import (
	"strings"
	"time"
)

// SupplierProduct is the reusable commercial identity of one article sold by
// one supplier: Supplier + supplier SKU. It is created or reused during
// purchase import and, once created, is never re-keyed by description
// similarity. Its optional ResourceID links it to a Resource Master entry;
// that relation may be added, changed, or removed later without touching
// any PurchaseLine's original data.
type SupplierProduct struct {
	ID          int64
	SupplierID  int64
	SupplierSKU string
	// Description is the last-seen description for this SKU. It is
	// informational only and never authoritative: each PurchaseLine keeps
	// its own original description regardless of this value.
	Description string
	ResourceID  *int64
	Notes       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// NewSupplierProductIdentity validates the identity fields required to
// create or reuse a SupplierProduct during import.
func NewSupplierProductIdentity(supplierID int64, sku, description string) (int64, string, string, error) {
	sku = strings.TrimSpace(sku)
	description = strings.TrimSpace(description)
	if supplierID <= 0 {
		return 0, "", "", NewValidationError("supplier_id", "must be positive")
	}
	if sku == "" {
		return 0, "", "", NewValidationError("supplier_sku", "is required")
	}
	return supplierID, sku, description, nil
}

// WithResource returns sp linked to resourceID. resourceID must be positive.
func (sp SupplierProduct) WithResource(resourceID int64) (SupplierProduct, error) {
	if resourceID <= 0 {
		return SupplierProduct{}, NewValidationError("resource_id", "must be positive")
	}
	sp.ResourceID = &resourceID
	return sp, nil
}

// WithoutResource returns sp with its Resource Master relation removed.
func (sp SupplierProduct) WithoutResource() SupplierProduct {
	sp.ResourceID = nil
	return sp
}
