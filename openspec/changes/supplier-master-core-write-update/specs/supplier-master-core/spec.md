# Delta for supplier-master-core

## ADDED Requirements

### Requirement: Update performs a full replace, not a patch

`UpdateSupplier` MUST replace all five `domain.SupplierDetails` content fields (`TradeName`, `LegalName`, `TaxIdentifier`, `Website`, `Notes`) wholesale via `Supplier.WithDetails`, which MUST always rebuild the record through a fresh `NewSupplier(details)` call. No partial-patch or field-merge logic MUST exist anywhere on the write path; an omitted or blank field on the request genuinely clears that field on the persisted record.

#### Scenario: Omitting an optional field clears it
- GIVEN a supplier with a non-empty `Notes` value
- WHEN `UpdateSupplier` is called with a `SupplierUpdateRequest` that omits `Notes`
- THEN the persisted supplier's `Notes` is empty afterward, not left unchanged

#### Scenario: All five content fields are replaced wholesale
- GIVEN a `SupplierUpdateRequest` with new values for all five content fields
- WHEN `UpdateSupplier` is called
- THEN the persisted supplier reflects exactly those five values, with none carried over from the prior record by merge logic

### Requirement: Inactive suppliers stay freely editable; Update never touches Active

`UpdateSupplier` MUST NOT apply an active-state gate: a deactivated supplier's details remain freely editable, mirroring internal behavior exactly. `UpdateSupplier` MUST NOT change the `Active` flag in either direction — it stays whatever it was before the call. This is a deliberate contract, not an oversight; a future change MUST NOT add an active-state gate or a reactivation side effect to `UpdateSupplier` without an explicit new requirement superseding this one.

#### Scenario: Updating an inactive supplier succeeds and leaves it inactive
- GIVEN a supplier with `Active == false`
- WHEN `UpdateSupplier` is called with valid content
- THEN the call succeeds, the content is updated, and the persisted `Active` value is still `false`

#### Scenario: Updating an active supplier never flips Active
- GIVEN a supplier with `Active == true`
- WHEN `UpdateSupplier` is called with valid content
- THEN the persisted `Active` value is still `true`; no field of the request influences it

## MODIFIED Requirements

### Requirement: Public write contract — Supplier Create and Update
The system SHALL expose a public, `internal`-free `Writer` over a `WriteCapabilities` port in `suppliercore`, declaring exactly two methods, `CreateSupplier` and `UpdateSupplier`. `NewWriter(nil)` SHALL return `INVALID_ARGUMENT`, never a nil-pointer panic.
(Previously: "Public write contract — Supplier Create only", declaring exactly one method.)

#### Scenario: External package creates a supplier
- GIVEN a Go test outside this module's `internal/` tree
- WHEN it imports `suppliercore`, builds a `Writer` via `NewWriter`, and calls `CreateSupplier`
- THEN it compiles, runs, and returns the persisted `Supplier` with no `internal` import

#### Scenario: External package updates a supplier
- GIVEN a Go test outside this module's `internal/` tree and an existing supplier
- WHEN it calls `UpdateSupplier` on the `Writer` with a valid `SupplierUpdateRequest`
- THEN it compiles, runs, and returns the persisted `Supplier` with no `internal` import

#### Scenario: Nil capability rejected at construction
- GIVEN a nil `WriteCapabilities`
- WHEN `NewWriter(nil)` is called
- THEN it returns a nil `*Writer` and `Code() == INVALID_ARGUMENT`

### Requirement: Shape validation is Actor-only for Create, Actor+ID for Update; content rules stay in the domain
`SupplierWriteRequest` validation SHALL check only that `Actor` is non-blank before delegating. `SupplierUpdateRequest` validation SHALL check that `Actor` is non-blank AND `ID > 0` before delegating. In both cases the five `domain.SupplierDetails` content fields are optional at the shape layer; their combined-content rule SHALL remain `domain.NewSupplier`'s sole authority. Neither request SHALL carry `Active`.
(Previously: "Shape validation is Actor-only; content rules stay in the domain" — covered Create only.)

#### Scenario: Blank Actor rejected before delegation
- GIVEN `Actor == ""` or all-whitespace on either request type
- WHEN `CreateSupplier` or `UpdateSupplier` is called
- THEN it returns `INVALID_ARGUMENT` without invoking the underlying capability

#### Scenario: Non-positive ID rejected on Update before delegation
- GIVEN a `SupplierUpdateRequest` with a non-blank `Actor` and `ID <= 0`
- WHEN `UpdateSupplier` is called
- THEN it returns `INVALID_ARGUMENT` without invoking the underlying capability

#### Scenario: Empty content reaches the domain, not a boundary rejection
- GIVEN a non-blank `Actor` (plus a positive `ID` for Update) and no trade name, legal name, or tax identifier
- WHEN `CreateSupplier` or `UpdateSupplier` is called
- THEN it delegates, and the domain's error surfaces as `Code() == VALIDATION`, not `INVALID_ARGUMENT`

### Requirement: Write-reachable error taxonomy, no new codes
The existing five-category `Error`/`ErrorCode` SHALL be reused. `CreateSupplier` SHALL make four reachable — `INVALID_ARGUMENT`, `VALIDATION`, `CONFLICT` (duplicate tax identifier), `INTERNAL` — with `NOT_FOUND` unreachable from Create. `UpdateSupplier` SHALL make all five reachable, including `NOT_FOUND` for an unknown ID (`GetSupplier` → `ErrSupplierNotFound`).
(Previously: "Create-reachable error taxonomy, no new codes" — declared `NOT_FOUND` permanently unreachable from the write surface.)

#### Scenario: Each Create-reachable category is proven
- GIVEN the four Create-applicable categories
- WHEN a scenario is built per category (blank Actor, empty content, duplicate tax identifier, unclassified failure)
- THEN each produces the matching `Code()`, and no test proves `NOT_FOUND` from Create

#### Scenario: All five categories are proven reachable from Update
- GIVEN blank Actor/non-positive ID, empty content, an unknown ID, a duplicate tax identifier, and an unclassified failure
- WHEN each is submitted through `UpdateSupplier`
- THEN each produces `INVALID_ARGUMENT`, `VALIDATION`, `NOT_FOUND`, `CONFLICT`, and `INTERNAL` respectively

### Requirement: No CAS; Actor is diagnostic-only; Update directly exercises the lost-update race
No `Revision`/CAS field SHALL be added to `SupplierWriteRequest`, `SupplierUpdateRequest`, or any entity. `UpdateSupplier` SHALL perform an unguarded read-then-write (`GetSupplier` → `WithDetails` → `repo.UpdateSupplier`, no transaction), so two concurrent updates to the same supplier silently last-write-win with zero error and zero signal; this MUST be disclosed in `doc.go`, not fixed, in this slice. `Actor` SHALL be required, passed via `internal/core.WithActor(ctx, actor)`, and SHALL NOT be persisted or returned on any DTO.
(Previously: "No CAS; Actor is diagnostic-only and a documented future audit seed" — described the race only as inherited/future-facing from Create.)

#### Scenario: No Revision field and no Actor leak
- GIVEN `SupplierWriteRequest`, `SupplierUpdateRequest`, and a successful call to either
- WHEN the request type and the returned `Supplier` are inspected
- THEN no `Revision`/`ExpectedRevision` field exists, and the returned `Supplier` carries no `Actor` field

#### Scenario: Concurrent updates silently last-write-win
- GIVEN two `UpdateSupplier` calls for the same `ID` racing without coordination
- WHEN both complete
- THEN the persisted record reflects only the later write, with no `CONFLICT` error or other signal raised by either call

### Requirement: Compiled write surface exports no ungraduated method
`WriteCapabilities` SHALL declare exactly two methods, `CreateSupplier` and `UpdateSupplier`; `Writer`'s exported method set SHALL contain no other method, per a reflection guard mirroring `resourcecore/writer_test.go:216`.
(Previously: declared exactly one method, `CreateSupplier`.)

#### Scenario: Reflection guard fails on any ungraduated method
- GIVEN the compiled `WriteCapabilities` and `Writer` types
- WHEN their method sets are enumerated via `reflect`
- THEN `WriteCapabilities.NumMethod() == 2`, and every exported `Writer` method is in `{CreateSupplier, UpdateSupplier}` — no Deactivate/Reactivate/HardDelete stub

### Requirement: No error leakage; defensive copying on the write path
`CreateSupplier` and `UpdateSupplier` SHALL NOT leak pgx/SQLSTATE/constraint/table/column detail through any public error, and SHALL defensively copy the inbound request and outbound `Supplier` so post-call mutation never affects a later read.
(Previously: covered `CreateSupplier` only.)

#### Scenario: Raw PostgreSQL error never reaches the public surface
- GIVEN an internal create or update fails with a raw, unwrapped `PgError`-shaped error
- WHEN it crosses the bridge into `suppliercore`
- THEN the public `Error` has `Code() == INTERNAL` with no SQLSTATE/constraint/table/column text

#### Scenario: Mutating the request after the call does not leak
- GIVEN a `SupplierWriteRequest` or `SupplierUpdateRequest` passed to its write method
- WHEN the caller mutates the request after the call returns
- THEN a subsequent `GetSupplier` for the affected record reflects the original request values
