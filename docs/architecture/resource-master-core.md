# Resource Master Core — Public Initialization

This guide records how `garfex.Open` composes the public `resourcecore` and `suppliercore` contracts over the existing authoritative services and PostgreSQL. It complements [Resource Master Source of Truth](catalog-source-of-truth.md).

## Decisions first

| Concern | Authoritative decision | Concrete boundary |
|---|---|---|
| Ownership | `resourcecore` is a translation boundary, not a second authority. It owns its DTOs and safe errors only; every read delegates to `internal/app/catalogo.Service` and `internal/app/recursos.Service` through a module-owned bridge. | `resourcecore/reader.go`, `internal/bridge/resourcecore/adapter.go` |
| Shipped operations | `Application` exposes Resource/Supplier Reader and Writer handles. Writers delegate graduated create, update, lifecycle, and hard-delete operations; no handle exposes a pool, repository, or authority. | `garfex.go`, `resourcecore/*.go`, `suppliercore/*.go` |
| Errors | Resource operation errors are `resourcecore.Error` values with one of fifteen stable `ErrorCode` values, mapped from Core through `internal/core.Map`. Supplier operations use `suppliercore.Error`; `Open` sanitizes bootstrap failures and preserves cancellation/deadline causes for `errors.Is`. No public error exposes PostgreSQL/driver detail. | `resourcecore/errors.go`, `suppliercore/errors.go`, `internal/core/errors.go`, `garfex.go` |
| Identity | `Resource.IdentityV1` is durable business identity. Catalog `CatalogRecord.ID`/`CatalogKey.ID` is an opaque, hash-derived reference only. | `resourcecore/types.go`, `internal/domain/resource_validation.go` |
| Freshness | Exactly one authoritative writer process. A `Reader` reflects a coherent snapshot as of its capability's last read; another process needs an explicit reload/restart to observe a write. There is no `Reload` on `Reader`. | `resourcecore/reader.go` |
| Migration 8 | Additive `revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0)` on `recursos` and all 11 catalog parent tables, backfilled to 1. Projected as `Revision uint64` wherever it exists; never derived from `updated_at`, `xmin`, or a hash ID. | `migrations/000008_resource_revisions.up.sql` |

## Authority boundaries

```text
external consumer -> garfex.Open(ctx, garfex.Config{DSN: ...})
  -> Application.{ResourceReader, ResourceWriter, SupplierReader, SupplierWriter}
  -> internal bridges -> authoritative application services -> PostgreSQL
```

- `Reader` never imports `internal`; it is constructed with any `ReadCapabilities` implementation, but the only implementation shipped and integration-tested is `internal/bridge/resourcecore.Adapter`.
- The adapter maps every public query to a domain call, translates errors through `internal/core.Map`, and defensively copies data at both edges — it holds no business rule of its own (lifecycle, presentation, dependency, or validation policy stays in `internal/app/catalogo` and `internal/app/recursos`).
- `garfex.Open` is the public composition root. It creates one private pgx pool, eagerly loads the catalog with the caller context, and shares one `CatalogAuthority` across catalog and resource services.

## Lifecycle semantics through READ

All 11 registered catalog kinds (`CLASE`, `FAMILIA`, `TIPO`, `CARACTERISTICA`, `CONJUNTO_OPCIONES`, `OPCION`, `RELACION_OPCIONES`, `UNIDAD`, `POLITICA_UNIDAD`, `APLICABILIDAD`, `PRESENTACION`) are lifecycle-capable in Core and project uniformly through `CatalogRecord{Active, Revision, Values}`; `APLICABILIDAD` additionally reports its complete ordered `Rules []ApplicabilityRule`. Deactivated records remain readable under an explicit `INACTIVE`/`ALL` lifecycle scope. Catalog hard delete is publicly shipped and removes a record from every lifecycle-scoped read; Resource hard delete is not publicly exposed.

## Errors

| Public category | Reachable from today's READ surface? |
|---|---|
| `INVALID_ARGUMENT` | Yes — request-shape validation (e.g. unknown kind, unsupported `ScopeAll`/`TypeCode` filter on resource search). |
| `NOT_FOUND` | Yes — absent catalog record or resource. |
| `INTEGRITY` | Yes — persisted structure/cardinality inconsistency surfaced while reading. |
| `INVALID_CATALOG` | Yes — the loaded catalog itself is structurally invalid. |
| `UNAVAILABLE` | Yes — context cancellation/deadline or a classified temporary outage. |
| `INTERNAL` | Yes — unclassified unexpected failure. |
| `DUPLICATE`, `INVALID_REFERENCE`, `VALIDATION`, `IDENTITY_CONFLICT`, `INVALID_LIFECYCLE`, `REACTIVATION_IMPOSSIBLE`, `IN_USE`, `IMMUTABLE_CODE`, `CONFLICT` | Writer outcomes, mapped by the existing bridge/service contracts when the corresponding operation reaches them. |

`resourcecore.Error` never implements `Unwrap`, never exposes a `Cause`, and its `Error()`/`%v`/`%+v` never contain a PostgreSQL message, SQLSTATE, constraint, table, column, or driver type. `internal/core.Map` performs one centralized, exhaustive translation from Core sentinels; the adapter never infers a category from `Error()`, a substring, or a PostgreSQL message.

## Writer topology and freshness

Exactly one process is the designated authoritative writer for Resource Master mutations; this package neither supports nor claims safe independent multi-process writers. A `Reader` reflects a coherent snapshot as of its backing capability's most recent read — there is no `LISTEN`/`NOTIFY`, polling, shared cache, or automatic cross-process refresh. A process other than the writer needs an explicit reconstruct/reload of its bridge, or a restart, to observe a write made elsewhere. `Reader` intentionally has no `Reload` method: publication control is not a consumer capability.

## Migration 8 compatibility

`migrations/000008_resource_revisions` adds an additive, backfilled `revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0)` column to `recursos` and the 11 catalog parent tables. It changes no `identity-v1` value and rewrites no existing row. A process running before this package or this migration existed continues to compile and read/write unaffected — the column is additive-only and no trigger derives it from `updated_at`, `xmin`, or a hash-derived catalog ID.

## Configuration and ownership

`Config.DSN` is required; `Open` does not read environment variables or `.env` files. pgx may still apply its documented `PG*` defaults for connection parameters omitted from the supplied DSN. Call `Application.Close` exactly when the consumer is done; it safely drains and closes the Core-owned pool. `Open` sanitizes bootstrap failures: malformed configuration and unclassified initialization failures use stable safe messages, while cancellation and deadline failures retain their standard causes for `errors.Is`. Resource operation errors retain the existing `resourcecore` mapping; Supplier operation errors use `suppliercore` errors.

```go
app, err := garfex.Open(ctx, garfex.Config{DSN: dsn})
if err != nil {
	return fmt.Errorf("open GARFEX Core: %w", err)
}
defer app.Close()
```

## Operator path: verify

1. Focused: `go test . -count=1`.
2. Style: `gofmt -l garfex.go garfex_test.go garfex_integration_test.go`.
3. Full suite: `go test ./... -count=1`.

Rollback of the public-composition change includes `garfex.go`, `garfex_test.go`, `garfex_integration_test.go`, `resourcecore/doc.go`, and this file. Reverting it does not alter schema, migrations, or PostgreSQL data.
