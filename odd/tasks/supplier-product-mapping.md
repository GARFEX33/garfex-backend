# Strengthen reusable SupplierProduct mapping

## Objective
Evolve the existing Purchase Core so `SupplierProduct` is the single source of
confirmed, reusable knowledge from `(SupplierID, SupplierSKU)` to the current
Resource Master, with optimistic concurrency, semantic transitions, append-only
audit, line-specific overrides, and derived effective resolution state.

## Problem / why
The current Core already reuses `SupplierProduct.ResourceID`, but link/unlink are
ambiguous last-write-wins operations, mapping and line-status writes are not one
transaction, `LinkStatus` mixes overrides with derived state, Resource activity is
not considered, audit is absent, and the existing upsert does not implement the
documented last-seen description behavior.

## Binding scope
- `SupplierProduct.CurrentMapping` contains confirmed knowledge only.
- Mapping identity is `(SupplierID, strings.TrimSpace(SupplierSKU))`; casing,
  punctuation, hyphens, and leading zeroes remain significant.
- `MappingRevision` changes only when confirmed reusable knowledge changes:
  confirm, correct, exceptional unlink, report identity conflict, or resolve it.
- Resource deactivate/reactivate changes only effective derived state through
  `Resource.Active`; it does not change MappingRevision or mapping audit.
- Mapping audit is append-only and records confirmed transitions only.
- `PurchaseLine` preserves XML facts and stores only its own resolution override;
  effective status derives from line + override + confirmed mapping + Resource.Active.
- No direct ResourceID is added to PurchaseLine.
- Import reuses only confirmed, active, conflict-free knowledge and performs no
  advanced resolution.
- No candidates, scores, confidence, DecisionProvider, LLM, approval workflow,
  event bus, outbox, or speculative future states/interfaces.
- Artifact/code/tests remain English; user conversation remains Spanish.
- No local `go build`; CI owns build.

## Design constraints
- `SupplierProduct` remains the aggregate root; current mapping is a value object,
  not a separately identified persisted entity.
- Persist one current mapping authority plus one append-only audit history.
- Persist line override, not PENDING/LINKED/SUSPENDED derived authority.
- PostgreSQL owns locking/CAS/commit mechanics, never business transition choice.
- Resource Master remains the sole authority for `Resource.Active` and its revision.
- Existing Purchase/XML fiscal facts, UUID deduplication, SupplierProductID indirection,
  public bounded-context separation, and retroactive history queries are preserved.

## Delivery strategy
- Strategy: `ask-on-risk`, resolved by the user to a feature-branch chain.
- Forecast: approximately 1,200–2,000 authored changed lines across domain, migration,
  PostgreSQL adapter, application, public contracts/bridge, and behavior/integration
  tests. This exceeds the ~400-line review threshold.
- Chain strategy: `feature-branch-chain`; planned slices are SP-1, SP-2/SP-3, and
  SP-4, with exact commit boundaries recorded as each work unit closes.
- Branch: `feat/supplier-product-mapping`.

## TDD
- Mode: strict TDD, explicitly selected by the user.
- Source: user choice after `.pi/` and `AGENTS.md` contained no TDD setting.
- Exact runner: each task's focused `go test` command below. Every behavioral unit must
  record observed RED, GREEN, and REFACTOR evidence; project checks remain required.

## Tasks

### SP-1 — Domain mapping and effective line resolution
Status: done.
Route: delegated writer; multi-file non-trivial write trigger.

- [x] Add current-mapping value objects, mapping revision/state/cause types, decision
      metadata, semantic transition validation, and append-only audit domain model.
- [x] Replace authoritative line LinkStatus semantics with line override plus pure
      effective-status resolution, preserving compatibility only as a derived read
      projection where required.
- [x] Add behavior-first domain tests for every valid/invalid transition, revision
      change rule, state/cause precedence, Resource activity, and line override.

Acceptance:
- MappingRevision changes only for the five authorized knowledge transitions.
- Resource activity and description updates never change MappingRevision.
- No candidate/provider/future-resolution concepts appear.

Focused checks:
- `go test ./internal/modules/purchases/domain -count=1`

Rollback boundary:
- Purchase-domain mapping/audit/override files and their focused tests only.

### SP-2 — Atomic persistence and semantic Core surface
Status: done.
Route: delegated writer; multi-file non-trivial write trigger.

- [x] Add reviewed migration support for mapping revision/conflict authority,
      append-only audit, and line override migration without a second effective-state
      authority.
- [x] Implement atomic PostgreSQL mapping transitions with row locking, CAS, audit
      insertion, and active-Resource validation.
- [x] Update SupplierProduct import upsert for last-seen description and return the
      confirmed/effective mapping projection without advanced resolution.
- [x] Derive effective line state in reads/import/history using Resource.Active;
      remove mapping-driven line-status cascades.
- [x] Add DB-free adapter tests and gated integration coverage for CAS, atomicity,
      audit, description, active/inactive resources, overrides, and retroactive history.
- [x] Replace ambiguous app/repository writes with semantic confirm, correct,
      exceptional unlink, report/resolve identity conflict, and line-override use cases.
- [x] Expose ExpectedRevision, explicit actor/origin/reason metadata, audit reads,
      effective status, and causes through purchasecore and its sole bridge.
- [x] Remove unsafe public writes rather than retaining wrappers that bypass CAS/audit.

Acceptance:
- No partial mapping-without-audit commit is possible.
- No line stores linked/pending/suspended as independent authority.
- Confirm/correct reject inactive targets atomically against Resource lifecycle.

Focused checks:
- `go test ./internal/modules/purchases/postgres -count=1`
- `go test ./internal/modules/purchases/app ./purchasecore ./internal/bridge/purchasecore -count=1`
- `go test ./migrations -count=1`

Runtime harness:
- PostgreSQL integration tests remain gated by the repository's existing DSN policy;
  no database or Docker action is authorized implicitly by this task document.

Rollback boundary:
- The new purchase mapping migration and Purchase PostgreSQL adapter/test changes.

### SP-3 — Application use cases and public Purchase Core contract
Status: merged into SP-2; no independent work-unit boundary.
Route: executed by the SP-2 delegated writer because persistence, semantic commands,
and removal of the legacy write path must land atomically to avoid an unsafe or
uncompilable intermediate commit.

- [x] Replace ambiguous link/unlink/status writes with semantic confirm, correct,
      exceptional unlink, report/resolve identity conflict, and line-override use cases.
- [x] Expose ExpectedRevision and explicit actor/origin/reason metadata.
- [x] Add mapping audit reads and effective line state/causes to public Purchase Core.
- [x] Update the sole purchasecore bridge and compile-time contract assertions.
- [x] Remove or explicitly retire public writes that cannot preserve CAS and semantic
      intent; do not retain unsafe wrappers.
- [x] Add application, public-contract, and bridge tests.

Acceptance:
- Public callers cannot write PENDING/LINKED/SUSPENDED directly.
- Future mechanisms can only converge on the same confirmed semantic commands.
- No transport types or hidden context metadata become business authority.

Focused checks:
- `go test ./internal/modules/purchases/app ./purchasecore ./internal/bridge/purchasecore -count=1`

Rollback boundary:
- Purchase app/public-contract/bridge changes and their tests.

### SP-4 — Composition, compatibility cleanup, and full verification
Status: done.
Route: delegated writer for remaining code/docs; verification routed separately under
orchestrator policy.

- [x] Wire the evolved Purchase Core without coupling Resource Master lifecycle writes
      to mapping revisions or audit.
- [x] Update repository-facing documentation whose public handles or mapping semantics
      are stale.
- [x] Verify no candidate/DecisionProvider/AI infrastructure entered the diff.
- [x] Run focused and full project checks and record every observed result.

Required checks:
- `gofmt -l .`
- `go vet ./...`
- `golangci-lint run ./...`
- `go test ./... -count=1`
- No local `go build`.

Runtime harness:
- N/A unless separately authorized gated PostgreSQL integration is required; this
  repository has no CLI/TUI/HTTP runtime boundary.

Rollback boundary:
- Composition/docs cleanup from this task; prior work units remain independently
  revertible.

## Progress
- Read-only domain and architecture analysis complete.
- Functional and technical scope approved by the user.
- Feature branch created.
- Strict TDD and `feature-branch-chain` were explicitly selected by the user.
- SP-1 domain implementation returned from the delegated writer within its allowed
  surfaces; persistence, application, public contracts, migrations, and wiring remain
  untouched.
- SP-1 implementation, verification, and work-unit commit are complete at
  `3ec662a32f40f4d38e71c100417e8ee4f04bea5a`.
- The SP-1 slice exceeds ~400 authored lines because the domain model, behavior tests,
  and five compile-preserving consumers are one smallest coherent green unit. Splitting
  before the compatibility adaptations leaves the repository uncompilable, so no
  size-only artificial split is applied.

## Verification evidence
SP-1 writer evidence:
- RED: `go test ./internal/modules/purchases/domain -count=1` failed on missing SP-1
  symbols (`MappingDecisionMetadata`, `SupplierProductMapping`, `LinkSuspended`, etc.).
- GREEN: the same command passed with 51 test entries.
- REFACTOR: gofmt followed by the same focused command; 51 test entries passed.
- Writer reported no candidate/provider/AI/workflow/event/outbox concepts introduced.
- Parent `git diff --check`: clean.
- Native risk assessment was unavailable (empty native output), so RDD-off policy
  treats SP-1 as high risk and requires an independent verifier.
- Independent verifier: focused package passed 51 tests, but SP-1 remains partial.
  Confirmed corrections: semantic transitions need expected-current-Resource guards;
  precedence/zero-value tests need expansion; `LinkSuspended` validity needs coverage.
- Parent readback additionally found that the audit entry duplicates Decision metadata
  fields and the aggregate retains an in-memory audit collection; both conflict with
  the intended separate append-only audit entity and must be simplified.
- The verifier's unresolved-vs-inactive precedence finding is rejected: without a
  ResourceID there is no Resource.Active dependency, so unresolved correctly precedes
  resource inactivity. Tests and comments must state this exact conditional rule.
- The legacy exported `LinkStatus` field is a temporary compatibility projection needed
  to keep later packages compiling in SP-1; SP-2 must remove it when persistence is
  migrated to `ResolutionOverride`, so it is not accepted as final architecture.
- SP-1 correction RED: focused tests failed because transition signatures lacked the
  newly required expected-current-Resource guards.
- SP-1 correction GREEN: the focused command passed 55 test entries; after refactor and
  gofmt it passed 57 entries.
- Correction removed aggregate-owned audit history and duplicated audit metadata,
  introduced expected Resource guards, made reason optional only for confirmation,
  renamed effective inactive mapping state to SUSPENDED, and expanded precedence and
  zero-value coverage.
- Parent post-correction `git diff --check`: clean. Native assessment remained
  unavailable, so independent re-verification was mandatory.
- Fresh independent verifier: PASS; focused domain command passed 57 test entries with
  no blocking findings. It confirmed the only remaining domain debt is the explicitly
  temporary `PurchaseLine.LinkStatus` compatibility projection assigned to SP-2.
- Parent spot check `env -u GARFEX_TEST_DSN -u GARFEX_ADMIN_TEST_DSN go test ./...
  -count=1`: FAILED at compile time because PostgreSQL scanners, the purchase bridge,
  and the app test repository still reference the removed `SupplierProduct.ResourceID`
  field/legacy helper methods. Domain packages passed; no runtime tests executed.
- SP-1 cannot close or commit while the branch fails to compile. A narrow compatibility
  adaptation will move those direct reads/fakes to `CurrentMapping.ResourceID` without
  reintroducing duplicate state or implementing SP-2 semantics early.
- The adaptation's compile RED also found one legacy fixture in
  `internal/bridge/purchasecore/adapter_test.go`. The user explicitly authorized adding
  exactly that test to the compatibility edit surfaces; no wider scope was granted.
- Compatibility GREEN: app, Purchase PostgreSQL, and purchase bridge focused packages
  passed after scanners/read projections/fakes moved to `CurrentMapping.ResourceID`.
- Writer full suite with integration DSNs unset: PASS. Post-gofmt focused rerun: PASS.
- No duplicate ResourceID field, migration, audit persistence, active-Resource guard,
  public contract change, or other SP-2 behavior was introduced.
- Parent post-adaptation `git diff --check`: clean. Current tracked diff is 181 additions
  and 60 deletions across 11 tracked files, plus four new domain files and this task doc.
- Native assessment remained unavailable, so a fresh independent verifier was required.
- Final independent verifier: PASS. Domain/app/Purchase PostgreSQL/bridge focused tests
  passed, and the full repository suite passed with integration DSNs unset. No tests
  were materially weakened and no blocking source finding remained.
- Parent final spot check `env -u GARFEX_TEST_DSN -u GARFEX_ADMIN_TEST_DSN go test
  ./... -count=1`: PASS across all repository packages.
- Runtime harness: N/A; SP-1 has no runtime boundary and database access stayed disabled.
- Rollback boundary: remove the four new domain mapping/resolution files and restore
  the eleven touched domain/compatibility files; SP-2 has not started.

- Post-commit verifier confirmed `3ec662a` is a valid closed boundary against
  `34ff871`; both focused and full DSN-unset suites passed and the tree stayed clean.
- SP-2 is now the sole active work unit. SP-3 was merged into the same vertical slice:
  committing persistence without the semantic app/public surface would leave either an
  unsafe legacy bypass or an uncompilable intermediate boundary.

SP-2/SP-3 writer evidence:
- Persistence RED was observed before implementation from missing mapping SQL/error
  symbols. Separate RED evidence was not captured for every later behavior group; this
  strict-TDD process gap is recorded honestly and cannot be reconstructed.
- Focused PostgreSQL, app/purchasecore/bridge, and migration suites passed; the full
  repository suite passed with integration DSNs unset; touched Go files were gofmt-clean
  and `git diff --check` was clean.
- Migration 000013, atomic CAS/audit paths, derived line resolution, last-seen import,
  semantic app/public commands, audit reads, and bridge mappings were implemented.
- DB-gated import/mapping integration scenarios skipped because database access was not
  authorized. Real migration and transaction behavior remain unverified.
- Current slice is approximately 982 tracked additions and 590 deletions plus seven new
  files; it is the smallest safe vertical boundary because persistence and removal of
  legacy public writes must land together.
- Native assessment was unavailable, so RDD-off policy required a fresh independent
  verifier before this work unit could close.
- Independent verifier: PARTIAL. All five authorized DSN-unset/static commands passed,
  but four findings block closure: inactive Resource state is discarded before the
  public SupplierProduct projection; the mapping integration test unconditionally skips
  even with a DSN; down migration maps inactive Resources to legacy VINCULADO; and
  cross-resource transitions lock expected/target Resources without canonical ordering.
- Accepted corrections: carry Resource.Active only as a non-authoritative read snapshot
  into effective public projection; remove the unconditional integration skip; down-map
  inactive Resources to legacy PENDIENTE; lock unique involved Resources in ascending ID
  order before validation to avoid cross-correction deadlocks.
- Correction RED: focused tests failed on missing `ResourceActive`, missing
  `orderedResourceIDs`, and the unsafe inactive down-projection. Correction GREEN:
  PostgreSQL passed 5 tests, app/public/bridge passed 49 tests, migrations passed 6 tests,
  and the full DSN-unset suite passed; formatting and diff checks were clean.
- The integration scenario now has only the repository DSN gate and covers migration
  presence, confirmation, stale CAS, audit persistence, inactive rollback, and unlink;
  it has not yet executed against PostgreSQL.
- Parent readback found and corrected one additional semantic issue: activity validation
  applied to current Resources as well as selected targets. Focused RED failed on missing
  target-only policy; GREEN passed 7 PostgreSQL tests and the full DSN-unset suite.
  Current inactive Resources may now be corrected away from, unlinked, conflict-reported,
  or resolved away from; only a newly selected target must be active.
- Fresh independent re-verification remained PARTIAL with two final source blockers:
  idempotent reconfirmation checked inactivity before recognizing a no-op, and integration
  cleanup violated the audit FK's `ON DELETE RESTRICT` order.
- Correction RED failed on missing no-op lock policy and audit cleanup SQL. GREEN passed
  9 PostgreSQL tests and the full DSN-unset suite. Idempotent reconfirmation now precedes
  target activity validation, and shared cleanup removes audit rows before SupplierProducts
  while tolerating pre-000013 schemas.
- Parent readback then caught a narrow no-op projection regression: successful inactive
  reconfirmation cleared its already-loaded ResourceActive snapshot. RED failed on missing
  refresh policy; GREEN passed 10 PostgreSQL tests and the full DSN-unset suite. Unchanged
  transitions now preserve the loaded snapshot; changed transitions refresh or clear it.
- Final independent source re-verification: PASS. PostgreSQL, app/public/bridge,
  migrations, full DSN-unset, and diff checks passed; all prior findings were closed.
- The user authorized one uniquely named disposable database in the existing local
  PostgreSQL service. First runtime run applied migrations 000001-000013 and passed the
  full Purchase PostgreSQL package, including import and mapping integration scenarios.
  The subsequent 000013 down succeeded, but re-up produced a genuine RED because the
  down migration left `supplier_products_mapping_resource_idx`; the disposable database
  was dropped and its absence confirmed.
- Runtime correction RED added an exact source assertion for the missing rollback index;
  GREEN added the matching `DROP INDEX IF EXISTS`. Migration tests passed 7 entries and
  the full DSN-unset suite remained green.
- A fresh disposable database then applied migrations 000001-000013 cleanly, passed the
  full Purchase PostgreSQL package, completed 000013 down/up cleanly, and passed the
  mapping integration scenario again after the round trip. The exact database was dropped
  and `pg_database` confirmed a remaining count of zero; no shared database, role, normal
  volume, or persistent fixture was modified.
- Parent final checks passed: `gofmt -l .` empty, `go vet ./...` clean,
  `golangci-lint run ./...` reported 0 issues, full DSN-unset tests passed, and
  `git diff --check` was clean. No local `go build` ran.
- Final independent correction verification: PASS. It matched the down/up index names,
  repeated all static suites successfully, found no remaining blocker, and accepted the
  runtime transcript explicitly as parent evidence rather than claiming to rerun it.
- SP-2/SP-3 committed as `651f87d5da0d91d950a5a92e4921e9a0ac66f3d5` with message
  `feat(purchases): persist supplier product mappings`; the worktree was clean afterward.
- Parent post-commit focused and full DSN-unset suites passed. Independent post-commit
  verifier: PASS; it confirmed the exact `3ec662a..651f87d` boundary, clean worktree,
  reviewed file scope, formatting, vet, lint, focused tests, migrations, and full suite.

SP-4 evidence:
- README now describes all three Core areas, lists the exact six public Application
  handles, and gives the concise Purchase mapping/derived-status contract with a pointer
  to `purchasecore/doc.go`.
- `TestOpenIntegrationExposesSixLiveHandles` now asserts both Purchase handles in the
  existing composition scenario; no new runtime behavior or DB requirement was added.
- Current Purchase source/public grep found no legacy generic link/unlink/status writes
  and no candidate, DecisionProvider, confidence, AI/LLM, approval, event bus, or outbox
  infrastructure. Historical ODD evidence was not misclassified as current API surface.
- Parent final checks passed: root and purchasecore/bridge focused tests, `gofmt -l .`,
  `go vet ./...`, `golangci-lint run ./...` (0 issues), full DSN-unset suite, and
  `git diff --check`.
- Independent SP-4 verifier: PASS. It confirmed the exact three-file scope, six-handle
  documentation/assertion, accurate task evidence, clean forbidden-concept audit, and
  all seven requested checks; SP-4 is safe to commit.

## Completion
SP-4 was committed with `docs(core): document purchase core composition`. The immediate
post-commit full DSN-unset suite passed and `git status --short` was empty. The approved
SupplierProduct mapping implementation is complete on `feat/supplier-product-mapping`;
no local `go build` ran and no disposable PostgreSQL database remains.
