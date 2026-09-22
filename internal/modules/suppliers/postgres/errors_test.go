package postgres

import (
	"errors"
	"testing"

	"github.com/GARFEX33/garfex-backend/internal/modules/suppliers/domain"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestMapWriteError(t *testing.T) {
	tests := []struct {
		name       string
		code       string
		constraint string
		want       error
	}{
		{name: "tax identifier conflict", code: "23505", constraint: "suppliers_tax_identifier_key", want: domain.ErrTaxIdentifierConflict},
		{name: "cross supplier branch", code: "23503", constraint: "supplier_contacts_supplier_branch_fkey", want: domain.ErrBranchOwnership},
		{name: "missing supplier", code: "23503", constraint: "supplier_branches_supplier_id_fkey", want: domain.ErrSupplierNotFound},
		{name: "check violation", code: "23514", constraint: "suppliers_meaningful", want: domain.ErrValidation},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mapWriteError("write", &pgconn.PgError{Code: tt.code, ConstraintName: tt.constraint, Message: "rejected"})
			if !errors.Is(err, tt.want) {
				t.Fatalf("mapWriteError() = %v, want %v", err, tt.want)
			}
		})
	}
}
