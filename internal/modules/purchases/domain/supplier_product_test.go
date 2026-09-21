package domain

import "testing"

func TestNewSupplierProductIdentity(t *testing.T) {
	tests := []struct {
		name       string
		supplierID int64
		sku        string
		wantErr    bool
		field      string
	}{
		{name: "valid identity", supplierID: 1, sku: "SKU-1"},
		{name: "trims sku", supplierID: 1, sku: "  SKU-1  "},
		{name: "invalid supplier id", supplierID: 0, sku: "SKU-1", wantErr: true, field: "supplier_id"},
		{name: "blank sku", supplierID: 1, sku: "  ", wantErr: true, field: "supplier_sku"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, sku, _, err := NewSupplierProductIdentity(tt.supplierID, tt.sku, "description")
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
			if sku != "SKU-1" {
				t.Fatalf("sku = %q, want trimmed SKU-1", sku)
			}
		})
	}
}

func TestNewSupplierProduct(t *testing.T) {
	sp, err := NewSupplierProduct(1, "  SKU-1  ", "description")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sp.SupplierSKU != "SKU-1" {
		t.Fatalf("sku = %q, want trimmed SKU-1", sp.SupplierSKU)
	}
	if sp.CurrentMapping.State(true) != MappingStateUnresolved {
		t.Fatalf("mapping state = %q, want unresolved", sp.CurrentMapping.State(true))
	}
}
