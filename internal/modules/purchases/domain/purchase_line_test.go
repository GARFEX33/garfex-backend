package domain

import (
	"testing"

	"github.com/shopspring/decimal"
)

func validLineDraft() PurchaseLineDraft {
	return PurchaseLineDraft{
		LineNumber:  1,
		Description: "cable THHN 10 AWG",
		SupplierSKU: "100299",
		Quantity:    decimal.NewFromInt(10),
		UnitPrice:   decimal.NewFromInt(5),
		Amount:      decimal.NewFromInt(50),
	}
}

func TestNewPurchaseLineDraft(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(l PurchaseLineDraft) PurchaseLineDraft
		wantErr bool
		field   string
	}{
		{name: "valid line succeeds", mutate: func(l PurchaseLineDraft) PurchaseLineDraft { return l }},
		{name: "non-positive line number", mutate: func(l PurchaseLineDraft) PurchaseLineDraft { l.LineNumber = 0; return l }, wantErr: true, field: "lines.line_number"},
		{name: "blank description", mutate: func(l PurchaseLineDraft) PurchaseLineDraft { l.Description = "  "; return l }, wantErr: true, field: "lines.description"},
		{name: "negative quantity", mutate: func(l PurchaseLineDraft) PurchaseLineDraft { l.Quantity = decimal.NewFromInt(-1); return l }, wantErr: true, field: "lines.quantity"},
		{name: "negative unit price", mutate: func(l PurchaseLineDraft) PurchaseLineDraft { l.UnitPrice = decimal.NewFromInt(-1); return l }, wantErr: true, field: "lines.unit_price"},
		{name: "negative amount", mutate: func(l PurchaseLineDraft) PurchaseLineDraft { l.Amount = decimal.NewFromInt(-1); return l }, wantErr: true, field: "lines.amount"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newPurchaseLineDraft(tt.mutate(validLineDraft()))
			if tt.wantErr {
				verr, ok := err.(ValidationError)
				if !ok {
					t.Fatalf("error = %v, want ValidationError", err)
				}
				if verr.Field != tt.field {
					t.Fatalf("field = %q, want %q", verr.Field, tt.field)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestPurchaseLineDraftHasSupplierIdentity(t *testing.T) {
	withSKU := validLineDraft()
	if !withSKU.HasSupplierIdentity() {
		t.Fatal("expected line with SKU to have supplier identity")
	}
	withoutSKU := validLineDraft()
	withoutSKU.SupplierSKU = ""
	if withoutSKU.HasSupplierIdentity() {
		t.Fatal("expected line without SKU to have no supplier identity")
	}
}
