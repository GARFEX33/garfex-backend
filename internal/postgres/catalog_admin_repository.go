package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/GARFEX33/garfex-costos-unitarios/internal/core"
	"github.com/GARFEX33/garfex-costos-unitarios/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// catalogAdminRepository implements domain.CatalogAdminRepository (design
// §3) generically over all 11 registered domain.CatalogKindCode values via
// the kindTables map (catalog_admin_kinds.go) — one Go map literal, no
// runtime information_schema introspection, matching design's explicit "not
// any arbitrary Postgres table" constraint.
type catalogAdminRepository struct{ pool *pgxpool.Pool }

// NewCatalogAdminRepository returns a domain.CatalogAdminRepository backed
// by pool, following the same construction convention as
// NewResourceRepository (resource_repository_crud.go).
func NewCatalogAdminRepository(pool *pgxpool.Pool) domain.CatalogAdminRepository {
	return &catalogAdminRepository{pool: pool}
}

// querier is the minimal pgx surface both *pgxpool.Pool and pgx.Tx satisfy —
// read paths (List/Get/Dependents/ReferencedByResources) accept either, so
// they can run standalone against the pool or, when convenient, inside a
// caller's own transaction.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// kindHandlers bundles every operation catalog_admin_kinds.go implements for
// one CatalogKind. dependents is a static list of RelationDescriptor-derived
// probe queries (design §6); referencedByResources is nil for the 4
// junction kinds that carry no ImmutableOnceReferenced código field
// (OptionRelation, UnitPolicy, AttributeBinding, PresentationField — see
// catalog_kind.go's own SoftDelete/junctionKinds documentation), in which
// case ReferencedByResources always reports false.
type kindHandlers struct {
	list                  func(ctx context.Context, q querier, f domain.CatalogFilter) ([]domain.CatalogRecord, error)
	get                   func(ctx context.Context, q querier, id int64) (domain.CatalogRecord, error)
	insert                func(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) (int64, error)
	update                func(ctx context.Context, tx pgx.Tx, rec domain.CatalogRecord) error
	setActive             func(ctx context.Context, tx pgx.Tx, id int64, active bool) error
	del                   func(ctx context.Context, tx pgx.Tx, id int64) error
	dependents            []dependencyProbe
	referencedByResources func(ctx context.Context, q querier, id int64) (bool, error)
}

// dependencyProbe is one row of the "what references this record" count
// used by Dependents (design §6, guarded-delete). query must select exactly
// one integer column: COUNT(*) scoped by the parent id ($1).
type dependencyProbe struct {
	kind     domain.CatalogKindCode
	query    string
	blocking bool
}

func (r *catalogAdminRepository) List(ctx context.Context, kind domain.CatalogKindCode, f domain.CatalogFilter) ([]domain.CatalogRecord, error) {
	h, ok := kindTables[kind]
	if !ok {
		return nil, fmt.Errorf("%w: %q", domain.ErrCatalogKindUnknown, kind)
	}
	return h.list(ctx, r.pool, f)
}

func (r *catalogAdminRepository) Get(ctx context.Context, kind domain.CatalogKindCode, id int64) (domain.CatalogRecord, error) {
	h, ok := kindTables[kind]
	if !ok {
		return domain.CatalogRecord{}, fmt.Errorf("%w: %q", domain.ErrCatalogKindUnknown, kind)
	}
	return h.get(ctx, r.pool, id)
}

func (r *catalogAdminRepository) Insert(ctx context.Context, rec domain.CatalogRecord) (int64, error) {
	h, ok := kindTables[rec.Kind]
	if !ok {
		return 0, fmt.Errorf("%w: %q", domain.ErrCatalogKindUnknown, rec.Kind)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin catalog insert: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id, err := h.insert(ctx, tx, rec)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit catalog insert: %w", err)
	}
	return id, nil
}

func (r *catalogAdminRepository) Update(ctx context.Context, rec domain.CatalogRecord) error {
	h, ok := kindTables[rec.Kind]
	if !ok {
		return fmt.Errorf("%w: %q", domain.ErrCatalogKindUnknown, rec.Kind)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin catalog update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := h.update(ctx, tx, rec); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit catalog update: %w", err)
	}
	return nil
}

func (r *catalogAdminRepository) SetActive(ctx context.Context, kind domain.CatalogKindCode, id int64, active bool) error {
	h, ok := kindTables[kind]
	if !ok {
		return fmt.Errorf("%w: %q", domain.ErrCatalogKindUnknown, kind)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin catalog set active: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := h.setActive(ctx, tx, id, active); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit catalog set active: %w", err)
	}
	return nil
}

func (r *catalogAdminRepository) Delete(ctx context.Context, kind domain.CatalogKindCode, id int64) error {
	h, ok := kindTables[kind]
	if !ok {
		return fmt.Errorf("%w: %q", domain.ErrCatalogKindUnknown, kind)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin catalog delete: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := h.del(ctx, tx, id); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit catalog delete: %w", err)
	}
	return nil
}

func (r *catalogAdminRepository) Dependents(ctx context.Context, kind domain.CatalogKindCode, id int64) ([]domain.CatalogDependency, error) {
	h, ok := kindTables[kind]
	if !ok {
		return nil, fmt.Errorf("%w: %q", domain.ErrCatalogKindUnknown, kind)
	}
	deps := make([]domain.CatalogDependency, 0, len(h.dependents))
	for _, probe := range h.dependents {
		var count int
		if err := r.pool.QueryRow(ctx, probe.query, id).Scan(&count); err != nil {
			return nil, fmt.Errorf("count dependents of %q for %q: %w", probe.kind, kind, err)
		}
		deps = append(deps, domain.CatalogDependency{Kind: probe.kind, Count: count, Blocking: probe.blocking})
	}
	return deps, nil
}

func (r *catalogAdminRepository) ReferencedByResources(ctx context.Context, kind domain.CatalogKindCode, id int64) (bool, error) {
	h, ok := kindTables[kind]
	if !ok {
		return false, fmt.Errorf("%w: %q", domain.ErrCatalogKindUnknown, kind)
	}
	if h.referencedByResources == nil {
		// Junction kinds (OptionRelation/UnitPolicy/AttributeBinding/
		// PresentationField) carry no código field, so Código immutability
		// never applies to them (catalog_kind.go's documented exemption).
		return false, nil
	}
	return h.referencedByResources(ctx, r.pool, id)
}

// --- shared field accessors (mirror domain/catalog_mutation.go's
// unexported text/boolean/integer/list/ref helpers, which this package
// cannot call directly) --------------------------------------------------

func fieldText(rec domain.CatalogRecord, name string) string { return rec.Values[name].Text }
func fieldBool(rec domain.CatalogRecord, name string) bool   { return rec.Values[name].Bool }
func fieldInt(rec domain.CatalogRecord, name string) int     { return rec.Values[name].Int }
func fieldList(rec domain.CatalogRecord, name string) []string {
	values := rec.Values[name].List
	if values == nil {
		return []string{}
	}
	return values
}
func fieldRef(rec domain.CatalogRecord, name string) string { return rec.Values[name].Ref.Code }

// --- shared record builders ----------------------------------------------

func textValue(s string) domain.CatalogValue { return domain.CatalogValue{Text: s} }
func boolValue(b bool) domain.CatalogValue   { return domain.CatalogValue{Bool: b} }
func intValue(i int) domain.CatalogValue     { return domain.CatalogValue{Int: i} }
func listValue(l []string) domain.CatalogValue {
	return domain.CatalogValue{List: l}
}
func refValue(kind domain.CatalogKindCode, code string) domain.CatalogValue {
	return domain.CatalogValue{Ref: domain.CatalogRef{Kind: kind, Code: code}}
}

func refValueLabel(kind domain.CatalogKindCode, code, label string) domain.CatalogValue {
	return domain.CatalogValue{Ref: domain.CatalogRef{Kind: kind, Code: code, Label: label}}
}

// --- error mapping ---------------------------------------------------------

// mapCatalogWriteError maps pgx constraint-violation codes surfaced by an
// Insert/Update to the catalog-structure sentinel errors (mirrors
// resource_repository_codec.go's mapRepositoryError for the resource-instance
// domain — kept separate per catalog_repository_errors.go's doc comment).
func mapCatalogWriteError(err error) error {
	core.Record(context.Background(), core.Operation("catalog.write"), "", 0, err)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf("%w: %s", domain.ErrCatalogDuplicate, pgErr.ConstraintName)
		case "23503", "23514", "23502":
			// 23502 (not-null violation) shows up when an INSERT ... SELECT
			// ref-resolution subquery finds nothing and a NOT NULL FK column
			// receives NULL — functionally the same "bad reference" case as
			// 23503/23514 for this repository's write shapes.
			return fmt.Errorf("%w: %s", domain.ErrCatalogReference, pgErr.Message)
		}
	}
	return err
}

// mapCatalogDeleteError maps a foreign-key violation on Delete to
// ErrCatalogInUse — the repository-level backstop behind the service/UI
// guarded-delete flow (design §6).
func mapCatalogDeleteError(err error) error {
	core.Record(context.Background(), core.Operation("catalog.delete"), "", 0, err)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return fmt.Errorf("%w: %s", domain.ErrCatalogInUse, pgErr.Message)
	}
	return err
}

// requireRow turns a zero-RowsAffected CommandTag (or pgx.ErrNoRows from a
// resolve-by-id SELECT) into domain.ErrCatalogRecordNotFound, uniformly
// across every kind's setActive/update/delete.
func requireRow(affected int64) error {
	if affected == 0 {
		return domain.ErrCatalogRecordNotFound
	}
	return nil
}

func isNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

// --- shared filter/query builders (used by every kind's list function in
// catalog_admin_kinds.go) --------------------------------------------------

// appendTextFilter adds an ILIKE-across-columns clause for CatalogFilter.Text
// (empty text or no columns is a no-op) — a single bound parameter is
// reused across every column via repeated positional references.
func appendTextFilter(conditions []string, args []any, text string, columns ...string) ([]string, []any) {
	if text == "" || len(columns) == 0 {
		return conditions, args
	}
	args = append(args, "%"+text+"%")
	idx := len(args)
	parts := make([]string, len(columns))
	for i, col := range columns {
		parts[i] = fmt.Sprintf("%s ILIKE $%d", col, idx)
	}
	return append(conditions, "("+strings.Join(parts, " OR ")+")"), args
}

func appendActiveFilter(conditions []string, status domain.CatalogStatus, column string) []string {
	switch status {
	case domain.CatalogStatusInactive:
		conditions = append(conditions, "NOT "+column)
	case domain.CatalogStatusAll:
		return conditions
	default:
		conditions = append(conditions, column)
	}
	return conditions
}

// appendEqualsFilter adds "column = $N" bound to value, unless value is empty.
func appendEqualsFilter(conditions []string, args []any, column, value string) ([]string, []any) {
	if value == "" {
		return conditions, args
	}
	args = append(args, value)
	return append(conditions, fmt.Sprintf("%s = $%d", column, len(args))), args
}

// parentRefCode reads a scoping ref code out of CatalogFilter.Parent (e.g.
// "class" when listing Familias under one Clase); "" means unscoped.
func parentRefCode(f domain.CatalogFilter, key string) string {
	return f.Parent[key].Ref.Code
}

// limitOffsetClause appends LIMIT/OFFSET when positive. Values are plain
// ints from CatalogFilter (never raw user text), so %d interpolation carries
// no injection risk — the same convention pgx itself has no placeholder
// syntax for LIMIT/OFFSET anyway.
func limitOffsetClause(f domain.CatalogFilter) string {
	clause := ""
	if f.Limit > 0 {
		clause += fmt.Sprintf(" LIMIT %d", f.Limit)
	}
	if f.Offset > 0 {
		clause += fmt.Sprintf(" OFFSET %d", f.Offset)
	}
	return clause
}

// finalizeQuery appends a WHERE clause (if any conditions), ORDER BY, and
// LIMIT/OFFSET to base.
func finalizeQuery(base string, conditions []string, orderBy string, f domain.CatalogFilter) string {
	if len(conditions) > 0 {
		base += " WHERE " + strings.Join(conditions, " AND ")
	}
	base += " ORDER BY " + orderBy
	base += limitOffsetClause(f)
	return base
}

// simpleSetActive/simpleDelete build the setActive/del kindHandlers funcs
// for the 7 kinds whose table has a real BIGSERIAL id column (Class, Family,
// Type, AttributeDefinition, Unit, OptionRelation, AttributeBinding) — table
// and idCol are always internal constants from kindTables, never derived
// from external input, so building the statement with fmt.Sprintf is safe.
func simpleSetActive(table, idCol string) func(ctx context.Context, tx pgx.Tx, id int64, active bool) error {
	return func(ctx context.Context, tx pgx.Tx, id int64, active bool) error {
		tag, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s SET active=$1, updated_at=NOW() WHERE %s=$2`, table, idCol), active, id)
		if err != nil {
			return mapCatalogWriteError(fmt.Errorf("set active on %s: %w", table, err))
		}
		return requireRow(tag.RowsAffected())
	}
}

func simpleDelete(table, idCol string) func(ctx context.Context, tx pgx.Tx, id int64) error {
	return func(ctx context.Context, tx pgx.Tx, id int64) error {
		tag, err := tx.Exec(ctx, fmt.Sprintf(`DELETE FROM %s WHERE %s=$1`, table, idCol), id)
		if err != nil {
			return mapCatalogDeleteError(fmt.Errorf("delete from %s: %w", table, err))
		}
		return requireRow(tag.RowsAffected())
	}
}
