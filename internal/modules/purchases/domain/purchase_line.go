package domain

import (
	"strings"

	"github.com/shopspring/decimal"
)

// PurchaseLine is one concept of a Purchase, preserved as a historical fact.
// Its original fields are never overwritten by later purchases or by
// classification/normalization work; only SupplierProductID and LinkStatus
// change after import, and only as an explicit relation correction.
type PurchaseLine struct {
	ID             int64
	PurchaseID     int64
	LineNumber     int
	Description    string
	SupplierSKU    string
	SATProductCode string
	Quantity       decimal.Decimal
	UnitCode       string
	Unit           string
	UnitPrice      decimal.Decimal
	Amount         decimal.Decimal
	Discount       decimal.Decimal
	TaxTransferred decimal.Decimal
	TaxWithheld    decimal.Decimal
	TaxObject      string

	SupplierProductID *int64
	LinkStatus        LinkStatus
}

// PurchaseLineDraft is the canonical, validated form of one concept about to
// be imported.
type PurchaseLineDraft struct {
	LineNumber     int
	Description    string
	SupplierSKU    string
	SATProductCode string
	Quantity       decimal.Decimal
	UnitCode       string
	Unit           string
	UnitPrice      decimal.Decimal
	Amount         decimal.Decimal
	Discount       decimal.Decimal
	TaxTransferred decimal.Decimal
	TaxWithheld    decimal.Decimal
	TaxObject      string
}

func newPurchaseLineDraft(line PurchaseLineDraft) (PurchaseLineDraft, error) {
	line.Description = strings.TrimSpace(line.Description)
	line.SupplierSKU = strings.TrimSpace(line.SupplierSKU)
	line.SATProductCode = strings.TrimSpace(line.SATProductCode)
	line.UnitCode = strings.TrimSpace(line.UnitCode)
	line.Unit = strings.TrimSpace(line.Unit)
	line.TaxObject = strings.TrimSpace(line.TaxObject)

	if line.LineNumber <= 0 {
		return PurchaseLineDraft{}, NewValidationError("lines.line_number", "must be positive")
	}
	if line.Description == "" {
		return PurchaseLineDraft{}, NewValidationError("lines.description", "is required")
	}
	if line.Quantity.IsNegative() {
		return PurchaseLineDraft{}, NewValidationError("lines.quantity", "must not be negative")
	}
	if line.UnitPrice.IsNegative() {
		return PurchaseLineDraft{}, NewValidationError("lines.unit_price", "must not be negative")
	}
	if line.Amount.IsNegative() {
		return PurchaseLineDraft{}, NewValidationError("lines.amount", "must not be negative")
	}
	return line, nil
}

// HasSupplierIdentity reports whether the line carries a supplier SKU
// reliable enough to identify a reusable SupplierProduct.
func (line PurchaseLineDraft) HasSupplierIdentity() bool {
	return line.SupplierSKU != ""
}
