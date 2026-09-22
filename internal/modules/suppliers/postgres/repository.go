// Package postgres implements Supplier Master persistence with PostgreSQL.
package postgres

import (
	"errors"
	"fmt"

	"github.com/GARFEX33/garfex-backend/internal/modules/suppliers/domain"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultLimit = 100

type repository struct{ pool *pgxpool.Pool }

type scanner interface{ Scan(...any) error }

func (r *repository) ready() error {
	if r.pool == nil {
		return errors.New("supplier repository: nil pool")
	}
	return nil
}

func nullableBool(value *bool) any {
	if value == nil {
		return nil
	}
	return *value
}

func limit(value int) int {
	if value <= 0 {
		return defaultLimit
	}
	return value
}

func limitPlusOne(value int) int { return limit(value) + 1 }

func offset(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func wrapRead(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func mapWriteError(operation string, err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			if pgErr.ConstraintName == "suppliers_tax_identifier_key" {
				return fmt.Errorf("%s: %w", operation, domain.ErrTaxIdentifierConflict)
			}
			return fmt.Errorf("%s: %w: %s", operation, domain.ErrConflict, pgErr.ConstraintName)
		case "23503":
			switch pgErr.ConstraintName {
			case "supplier_contacts_supplier_branch_fkey":
				return fmt.Errorf("%s: %w", operation, domain.ErrBranchOwnership)
			case "supplier_branches_supplier_id_fkey", "supplier_contacts_supplier_id_fkey":
				return fmt.Errorf("%s: %w", operation, domain.ErrSupplierNotFound)
			}
			return fmt.Errorf("%s: %w: %s", operation, domain.ErrValidation, pgErr.Message)
		case "23502", "23514":
			return fmt.Errorf("%s: %w: %s", operation, domain.ErrValidation, pgErr.Message)
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}
