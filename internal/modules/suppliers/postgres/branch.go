package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/GARFEX33/garfex-backend/internal/modules/suppliers/domain"
	"github.com/jackc/pgx/v5"
)

const branchColumns = `id, supplier_id, name, branch_reference, city, state, country, address, general_phone, general_email, notes, active, created_at, updated_at`

const createBranchSQL = `INSERT INTO public.supplier_branches (supplier_id, name, branch_reference, city, state, country, address, general_phone, general_email, notes, active)
	VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING ` + branchColumns

const getBranchSQL = `SELECT ` + branchColumns + ` FROM public.supplier_branches WHERE supplier_id = $1 AND id = $2`

const listBranchesSQL = `SELECT ` + branchColumns + ` FROM public.supplier_branches
	WHERE supplier_id = $1
	  AND ($2::BOOLEAN IS NULL OR active = $2)
	  AND ($3 = '' OR name ILIKE '%' || $3 || '%' OR branch_reference ILIKE '%' || $3 || '%' OR city ILIKE '%' || $3 || '%' OR state ILIKE '%' || $3 || '%' OR country ILIKE '%' || $3 || '%' OR address ILIKE '%' || $3 || '%' OR general_phone ILIKE '%' || $3 || '%' OR general_email ILIKE '%' || $3 || '%' OR notes ILIKE '%' || $3 || '%')
	ORDER BY lower(city), lower(name), id LIMIT $4 OFFSET $5`

func buildListBranchesQuery(supplierID int64, criteria domain.ListCriteria) (string, []any) {
	return listBranchesSQL, []any{supplierID, nullableBool(criteria.Active), criteria.Text, limitPlusOne(criteria.Limit), offset(criteria.Offset)}
}

func (r *repository) CreateBranch(ctx context.Context, value domain.Branch) (domain.Branch, error) {
	if err := r.ready(); err != nil {
		return domain.Branch{}, err
	}
	created, err := scanBranch(r.pool.QueryRow(ctx, createBranchSQL, value.SupplierID, value.Name, value.Reference, value.City, value.State, value.Country, value.Address, value.GeneralPhone, value.GeneralEmail, value.Notes, value.Active))
	return created, mapWriteError("create branch", err)
}

func (r *repository) GetBranch(ctx context.Context, supplierID, id int64) (domain.Branch, error) {
	if err := r.ready(); err != nil {
		return domain.Branch{}, err
	}
	value, err := scanBranch(r.pool.QueryRow(ctx, getBranchSQL, supplierID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Branch{}, fmt.Errorf("%w: supplier %d, branch %d", domain.ErrBranchNotFound, supplierID, id)
	}
	return value, wrapRead("get branch", err)
}

func (r *repository) ListBranches(ctx context.Context, supplierID int64, criteria domain.ListCriteria) ([]domain.Branch, error) {
	if err := r.ready(); err != nil {
		return nil, err
	}
	query, args := buildListBranchesQuery(supplierID, criteria)
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list branches: %w", err)
	}
	defer rows.Close()
	values := make([]domain.Branch, 0)
	for rows.Next() {
		value, err := scanBranch(rows)
		if err != nil {
			return nil, fmt.Errorf("scan branch: %w", err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read branches: %w", err)
	}
	return values, nil
}

func (r *repository) UpdateBranch(ctx context.Context, value domain.Branch) (domain.Branch, error) {
	if err := r.ready(); err != nil {
		return domain.Branch{}, err
	}
	const query = `UPDATE public.supplier_branches SET name=$3, branch_reference=$4, city=$5, state=$6, country=$7, address=$8, general_phone=$9, general_email=$10, notes=$11 WHERE supplier_id=$1 AND id=$2 RETURNING ` + branchColumns
	updated, err := scanBranch(r.pool.QueryRow(ctx, query, value.SupplierID, value.ID, value.Name, value.Reference, value.City, value.State, value.Country, value.Address, value.GeneralPhone, value.GeneralEmail, value.Notes))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Branch{}, fmt.Errorf("%w: supplier %d, branch %d", domain.ErrBranchNotFound, value.SupplierID, value.ID)
	}
	return updated, mapWriteError("update branch", err)
}

func (r *repository) SetBranchActive(ctx context.Context, supplierID, id int64, active bool) (domain.Branch, error) {
	if err := r.ready(); err != nil {
		return domain.Branch{}, err
	}
	const query = `UPDATE public.supplier_branches SET active=$3 WHERE supplier_id=$1 AND id=$2 RETURNING ` + branchColumns
	updated, err := scanBranch(r.pool.QueryRow(ctx, query, supplierID, id, active))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Branch{}, fmt.Errorf("%w: supplier %d, branch %d", domain.ErrBranchNotFound, supplierID, id)
	}
	return updated, mapWriteError("set branch active", err)
}

func scanBranch(row scanner) (domain.Branch, error) {
	var value domain.Branch
	err := row.Scan(&value.ID, &value.SupplierID, &value.Name, &value.Reference, &value.City, &value.State, &value.Country, &value.Address, &value.GeneralPhone, &value.GeneralEmail, &value.Notes, &value.Active, &value.CreatedAt, &value.UpdatedAt)
	return value, err
}
