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

func TestSupplierProductWithResource(t *testing.T) {
	sp := SupplierProduct{ID: 1, SupplierID: 1, SupplierSKU: "SKU-1"}

	linked, err := sp.WithResource(42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if linked.ResourceID == nil || *linked.ResourceID != 42 {
		t.Fatalf("resource id = %v, want 42", linked.ResourceID)
	}

	if _, err := sp.WithResource(0); err == nil {
		t.Fatal("expected error for non-positive resource id")
	}

	unlinked := linked.WithoutResource()
	if unlinked.ResourceID != nil {
		t.Fatalf("resource id = %v, want nil after unlink", unlinked.ResourceID)
	}
}
