// Package postgres implements Purchase and Price History persistence with
// PostgreSQL.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/GARFEX33/garfex-costos-unitarios/internal/modules/purchases/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultLimit = 100

type repository struct{ pool *pgxpool.Pool }

// scanner is satisfied by pgx.Row and pgx.Rows.
type scanner interface{ Scan(...any) error }

// querier is satisfied by *pgxpool.Pool and pgx.Tx, so read helpers can run
// standalone or inside the Import transaction without duplication.
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func (r *repository) ready() error {
	if r.pool == nil {
		return errors.New("purchase repository: nil pool")
	}
	return nil
}

func limit(value int) int {
	if value <= 0 {
		return defaultLimit
	}
	return value
}

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
		case "23503":
			return fmt.Errorf("%s: %w: %s", operation, domain.ErrValidation, pgErr.ConstraintName)
		case "23502", "23514":
			return fmt.Errorf("%s: %w: %s", operation, domain.ErrValidation, pgErr.Message)
		case "23505":
			return fmt.Errorf("%s: %w: %s", operation, domain.ErrConflict, pgErr.ConstraintName)
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func notFound(err error, wrapped error) error {
	if isNoRows(err) {
		return wrapped
	}
	return err
}

func isNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
