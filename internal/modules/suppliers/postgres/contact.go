package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/GARFEX33/garfex-backend/internal/modules/suppliers/domain"
	"github.com/jackc/pgx/v5"
)

const contactColumns = `id, supplier_id, branch_id, name, role, phone, mobile, email, notes, active, created_at, updated_at`

const createContactSQL = `INSERT INTO public.supplier_contacts (supplier_id, branch_id, name, role, phone, mobile, email, notes, active)
	VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING ` + contactColumns

const getContactSQL = `SELECT ` + contactColumns + ` FROM public.supplier_contacts WHERE supplier_id = $1 AND id = $2`

const listContactsSQL = `SELECT ` + contactColumns + ` FROM public.supplier_contacts
	WHERE supplier_id = $1
	  AND ($2::BOOLEAN IS NULL OR active = $2)
	  AND ($3::BIGINT IS NULL OR branch_id = $3)
	  AND ($4 = '' OR name ILIKE '%' || $4 || '%' OR role ILIKE '%' || $4 || '%' OR phone ILIKE '%' || $4 || '%' OR mobile ILIKE '%' || $4 || '%' OR email ILIKE '%' || $4 || '%' OR notes ILIKE '%' || $4 || '%')
	ORDER BY lower(name), id LIMIT $5 OFFSET $6`

func buildListContactsQuery(supplierID int64, criteria domain.ContactListCriteria) (string, []any) {
	return listContactsSQL, []any{supplierID, nullableBool(criteria.Active), criteria.BranchID, criteria.Text, limitPlusOne(criteria.Limit), offset(criteria.Offset)}
}

func (r *repository) CreateContact(ctx context.Context, value domain.Contact) (domain.Contact, error) {
	if err := r.ready(); err != nil {
		return domain.Contact{}, err
	}
	created, err := scanContact(r.pool.QueryRow(ctx, createContactSQL, value.SupplierID, value.BranchID, value.Name, value.Role, value.Phone, value.Mobile, value.Email, value.Notes, value.Active))
	return created, mapWriteError("create contact", err)
}

func (r *repository) GetContact(ctx context.Context, supplierID, id int64) (domain.Contact, error) {
	if err := r.ready(); err != nil {
		return domain.Contact{}, err
	}
	value, err := scanContact(r.pool.QueryRow(ctx, getContactSQL, supplierID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Contact{}, fmt.Errorf("%w: supplier %d, contact %d", domain.ErrContactNotFound, supplierID, id)
	}
	return value, wrapRead("get contact", err)
}

func (r *repository) ListContacts(ctx context.Context, supplierID int64, criteria domain.ContactListCriteria) ([]domain.Contact, error) {
	if err := r.ready(); err != nil {
		return nil, err
	}
	query, args := buildListContactsQuery(supplierID, criteria)
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list contacts: %w", err)
	}
	defer rows.Close()
	values := make([]domain.Contact, 0)
	for rows.Next() {
		value, err := scanContact(rows)
		if err != nil {
			return nil, fmt.Errorf("scan contact: %w", err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read contacts: %w", err)
	}
	return values, nil
}

func (r *repository) UpdateContact(ctx context.Context, value domain.Contact) (domain.Contact, error) {
	if err := r.ready(); err != nil {
		return domain.Contact{}, err
	}
	const query = `UPDATE public.supplier_contacts SET branch_id=$3, name=$4, role=$5, phone=$6, mobile=$7, email=$8, notes=$9 WHERE supplier_id=$1 AND id=$2 RETURNING ` + contactColumns
	updated, err := scanContact(r.pool.QueryRow(ctx, query, value.SupplierID, value.ID, value.BranchID, value.Name, value.Role, value.Phone, value.Mobile, value.Email, value.Notes))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Contact{}, fmt.Errorf("%w: supplier %d, contact %d", domain.ErrContactNotFound, value.SupplierID, value.ID)
	}
	return updated, mapWriteError("update contact", err)
}

func (r *repository) SetContactActive(ctx context.Context, supplierID, id int64, active bool) (domain.Contact, error) {
	if err := r.ready(); err != nil {
		return domain.Contact{}, err
	}
	const query = `UPDATE public.supplier_contacts SET active=$3 WHERE supplier_id=$1 AND id=$2 RETURNING ` + contactColumns
	updated, err := scanContact(r.pool.QueryRow(ctx, query, supplierID, id, active))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Contact{}, fmt.Errorf("%w: supplier %d, contact %d", domain.ErrContactNotFound, supplierID, id)
	}
	return updated, mapWriteError("set contact active", err)
}

func scanContact(row scanner) (domain.Contact, error) {
	var value domain.Contact
	err := row.Scan(&value.ID, &value.SupplierID, &value.BranchID, &value.Name, &value.Role, &value.Phone, &value.Mobile, &value.Email, &value.Notes, &value.Active, &value.CreatedAt, &value.UpdatedAt)
	return value, err
}
