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
- Chain strategy: `feature-branch-chain`; planned slices are SP-1, SP-2, and SP-3/SP-4,
  with exact commit boundaries recorded as each work unit closes.
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

### SP-2 — Atomic persistence, migration, and import projection
Status: pending.
Route: delegated writer; multi-file non-trivial write trigger.

- [ ] Add reviewed migration support for mapping revision/conflict authority,
      append-only audit, and line override migration without a second effective-state
      authority.
- [ ] Implement atomic PostgreSQL mapping transitions with row locking, CAS, audit
      insertion, and active-Resource validation.
- [ ] Update SupplierProduct import upsert for last-seen description and return the
      confirmed/effective mapping projection without advanced resolution.
- [ ] Derive effective line state in reads/import/history using Resource.Active;
      remove mapping-driven line-status cascades.
- [ ] Add DB-free adapter tests and gated integration coverage for CAS, atomicity,
      audit, description, active/inactive resources, overrides, and retroactive history.

Acceptance:
- No partial mapping-without-audit commit is possible.
- No line stores linked/pending/suspended as independent authority.
- Confirm/correct reject inactive targets atomically against Resource lifecycle.

Focused checks:
- `go test ./internal/modules/purchases/postgres -count=1`
- `go test ./migrations -count=1`

Runtime harness:
- PostgreSQL integration tests remain gated by the repository's existing DSN policy;
  no database or Docker action is authorized implicitly by this task document.

Rollback boundary:
- The new purchase mapping migration and Purchase PostgreSQL adapter/test changes.

### SP-3 — Application use cases and public Purchase Core contract
Status: pending.
Route: delegated writer; multi-file non-trivial write trigger.

- [ ] Replace ambiguous link/unlink/status writes with semantic confirm, correct,
      exceptional unlink, report/resolve identity conflict, and line-override use cases.
- [ ] Expose ExpectedRevision and explicit actor/origin/reason metadata.
- [ ] Add mapping audit reads and effective line state/causes to public Purchase Core.
- [ ] Update the sole purchasecore bridge and compile-time contract assertions.
- [ ] Remove or explicitly retire public writes that cannot preserve CAS and semantic
      intent; do not retain unsafe wrappers.
- [ ] Add application, public-contract, and bridge tests.

Acceptance:
- Public callers cannot write PENDING/LINKED/SUSPENDED directly.
- Future mechanisms can only converge on the same confirmed semantic commands.
- No transport types or hidden context metadata become business authority.

Focused checks:
- `go test ./internal/modules/purchases/app ./purchasecore ./internal/bridge/purchasecore -count=1`

Rollback boundary:
- Purchase app/public-contract/bridge changes and their tests.

### SP-4 — Composition, compatibility cleanup, and full verification
Status: pending.
Route: delegated writer for remaining code/docs; verification routed separately under
orchestrator policy.

- [ ] Wire the evolved Purchase Core without coupling Resource Master lifecycle writes
      to mapping revisions or audit.
- [ ] Update repository-facing documentation whose public handles or mapping semantics
      are stale.
- [ ] Verify no candidate/DecisionProvider/AI infrastructure entered the diff.
- [ ] Run focused and full project checks and record every observed result.

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
- SP-1 implementation and independent verification are complete; its work-unit commit
  is the remaining closure action.
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

## Next step
Create the SP-1 conventional work-unit commit, record its identity, then start SP-2.
