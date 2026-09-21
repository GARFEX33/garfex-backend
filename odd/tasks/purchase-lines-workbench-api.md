# Publish the purchase-line workbench API

Expose one server-authoritative purchase-line workbench and one manual resolution action so the frontend can review purchases across documents without client-side aggregation.

## Ownership

| Area | Owner | Boundary |
|---|---|---|
| HTTP routes, query decoding, response mapping, OpenAPI, handler tests | `garfex-api` | Implemented in this repository |
| Cross-document read model and transactional line resolution | Purchase Core owner | Requested from `garfex-costos-unitarios-workspace`; this task does not edit Core |
| Workbench UI and resolution dialog | Frontend owner | Out of scope |

## API contract

### Read workbench

`GET /v1/purchase-lines`

Supported query parameters:

- `supplierId`
- `status` (effective status)
- `dateFrom`
- `dateTo`
- `invoice`
- `supplierSku` (immutable XML evidence)
- `description`
- `limit`
- `offset`

Rows are ordered by purchase `issuedAt DESC`, then purchase-line id `DESC`. The response includes fiscal document identity, supplier/resource presentation, XML and commercial SKU identities, monetary fields, current mapping revision, stored resolution override, and effective status/cause.

### Resolve one line

`POST /v1/purchase-lines/{lineId}/resolve`

The handler delegates to one transactional Core command. It never creates a direct `PurchaseLine -> Resource` authority.

- An already-associated line supplies its expected SupplierProduct id and mapping revision.
- An unassociated line supplies a real, stable `commercialSupplierSku`; Core derives the supplier from the purchase, creates or reuses the SupplierProduct, associates the line, and confirms the SupplierProduct mapping atomically.
- `PurchaseLine.supplierSku` remains immutable XML evidence.
- Without a stable commercial identity, the line remains pending.

After success or conflict, clients reread the workbench. Ambiguous failures are never retried blindly.

### Published contract decisions

- The OpenAPI document mirrors the current HTTP handlers exactly: the workbench
  row is closed and contains fiscal identity, immutable XML SKU, nullable
  commercial/mapping/resource projections, monetary strings, and effective
  resolution state.
- Mapping commands are separate optimistic-concurrency operations. Confirm and
  resolve-conflict require `resourceId`; correct and resolve-conflict also
  require `expectedCurrentResourceId`; retire and report-conflict require only
  the expected current Resource. Every command carries `actor`, `reason`, and
  `expectedRevision` and returns the authoritative `SupplierProduct`.
- Line resolution requires the nullable expected SupplierProduct and mapping
  snapshots. `commercialSupplierSku` is meaningful only for a null expected
  SupplierProduct; the XML `supplierSku` is never mutated. The response includes
  `line`, `supplierProduct`, and `commercialIdentity` with `CREATED`, `REUSED`,
  or `ALREADY_MAPPED` disposition.
- PurchaseLine, history, and import schemas expose
  `resolutionRevision`, `resolutionOverride`, `effectiveStatus`, and
  `effectiveCause`; SupplierProduct exposes mapping/resource state projections.
  All published purchase schemas are closed with `additionalProperties: false`
  and complete required lists.
- Purchase-specific 404/409/422 machine outcomes are documented with the
  existing `{error, code, detail?}` response shape. Clients reread authoritative
  state after success, conflict, or ambiguous failure and never retry blindly.
  Legacy `/link`, `/unlink`, and `/link-status` paths and request schemas are
  removed from OpenAPI because the runtime returns 404.

### Replace legacy purchase mutations

The new Core mapping model intentionally replaces, rather than emulates, the old link/status commands. The API publishes these explicit manual transitions:

- `POST /v1/supplier-products/{id}/mapping/confirm`
- `POST /v1/supplier-products/{id}/mapping/correct`
- `POST /v1/supplier-products/{id}/mapping/retire`
- `POST /v1/supplier-products/{id}/mapping/report-conflict`
- `POST /v1/supplier-products/{id}/mapping/resolve-conflict`
- `POST /v1/purchase-lines/{lineId}/resolution-override`

Every mapping transition carries its expected revision and, where applicable, expected current Resource id. Only `NONE`, `NO_APLICA`, and `CONFLICTO` are valid line overrides; pending, linked, and suspended are derived states.

The router and OpenAPI remove the legacy `/link`, `/unlink`, and `/link-status` operations. Backend and frontend coordinate the cut so an incompatible Core/API is not deployed while the frontend still calls legacy routes.

## Core dependency

Requested capabilities:

1. A paginated cross-document purchase-line query with authoritative filtering and display projections.
2. A single transactional line-resolution command that reuses SupplierProduct mapping rules and audit semantics.
3. Public typed validation, not-found, stale-revision, and state-conflict errors suitable for HTTP mapping.

The Core dependency is available from the owner branch at commits `35bc85f99063` and incremental `59ae0e4c350f`, tracked by issue #188 and PR https://github.com/GARFEX33/garfex-costos-unitarios/pull/189. The API pins the published pseudo-version `v0.4.1-0.20260921042807-59ae0e4c350f`. No Core files, migrations, database, or service runtime are modified here.

## Work units

### 1. Cross-document read route

- Extend the narrow purchase reader capability.
- Decode and validate all server-side filters.
- Map Core workbench rows without N+1 reads.
- Publish OpenAPI schema and handler coverage.
- Verify focused package tests and full repository tests.

### 2. Manual mapping and resolution routes

- Extend the narrow purchase writer capability.
- Decode mutually exclusive existing/new commercial-identity request shapes.
- Publish every explicit mapping transition and the valid line-override transition.
- Remove legacy mutation paths rather than silently adapting them.
- Map Core error codes to explicit HTTP outcomes.
- Return authoritative line and SupplierProduct projections.
- Publish OpenAPI schemas and handler coverage.
- Verify focused package tests and full repository tests.

## Review workload decision

The user accepted one cohesive `size:exception` delivery. The approximately 2,150-line change is dominated by closed OpenAPI schemas and tests, and no slice under 400 lines preserves compile and published-contract coherence. The earlier two-commit forecast is replaced by this single forthcoming behavior commit.

Rollback boundary: revert that one API behavior commit covering runtime, routes, OpenAPI, tests, documentation, and the dependency pin. The Core PR remains a separate rollback and delivery boundary.

## Final implementation and delivery evidence

- Implemented `GET /v1/purchase-lines` workbench retrieval.
- Implemented all five mapping routes, line resolution, and the audited optimistic resolution override.
- Legacy purchase mutation routes return 404 and are removed from OpenAPI.
- The API pins published Core pseudo-version `v0.4.1-0.20260921042807-59ae0e4c350f`.
- Behavior commit recorded: `c38eba6` (`feat(httpapi): add purchase line workbench`).
- Frontend integration-ready signal recorded; coordinated deployment remains blocked until the frontend migrates its legacy calls.

## Verification evidence

| Check | Result |
|---|---|
| `go test ./... -count=1` | PASS — 562 tests |
| `go test -race ./... -count=1` | PASS — 562 tests |
| `GOWORK=off go test ./... -count=1` | PASS — 562 tests |
| `go vet ./...` | PASS |
| `GOWORK=off go vet ./...` | PASS |
| `git diff --check` | PASS — clean |

Both normal `go.work` and standalone `GOWORK=off` compile/test paths pass. The standalone dependency blocker was resolved by the published Core pin.

Runtime harness is N/A: no configured GARFEX API/Core database DSN exists. The HTTP boundary was verified with `httptest`; Core structural and migration tests exist, but live PostgreSQL execution was skipped. This record does not claim live database runtime coverage.

Independent verification passed after the strict-body and OpenAPI boundary fixes, including all normal, `GOWORK=off`, race, vet, and diff checks listed above. Native review was approved and acknowledged for lineage `review-f3b5d9069b992b1d`, target `sha256:9e5f89a27b2785d21acc09d7e54589b8534bbdb965dfeaa45cd8230ab8c149ab`. After dependency and documentation follow-ups, a final native-review restart was unavailable with a schema-incompatible/no-lineage result; this does not replace the earlier substantive approved/acknowledged review. The one informational leading-zero `supplierId` warning was fixed and covered afterward.

## Completion checklist

- [x] Core public capability is available and reviewed read-only from this repository.
- [x] Read route and contract are implemented.
- [x] Resolution route and contract are implemented.
- [x] Focused and full tests pass.
- [x] Native review outcome is recorded.
- [x] Rollback boundary is recorded.
- [x] API behavior commit identity is recorded: `c38eba6` (`feat(httpapi): add purchase line workbench`).
- [x] Frontend owner receives an explicit integration-ready signal; coordinated deployment remains blocked until the frontend migrates legacy calls.
