# Consolidate Backend — Phase 1

## Objective

Consolidate the existing GARFEX HTTP API and Core into one self-contained Go backend repository while preserving Core authority, public contracts, API behavior, PostgreSQL persistence, migrations, and source history.

## Problem

The API and Core currently live in separate repositories. The API module pins a published Core pseudo-version while local development uses `go.work` to resolve the sibling Core checkout. This can produce local/CI dependency drift and prevents building the complete backend from one clone.

## Why

A single backend repository must support the HTTP API and future worker executables against the same Core without duplicating business behavior or requiring external GARFEX module checkouts.

## Scope

- Preserve the current Core repository as the consolidation base.
- Import the tracked API source and relevant Git history.
- Place the executable at `cmd/api` and the HTTP adapter at `internal/httpapi`.
- Use one root `go.mod` and `go.sum`.
- Remove the API's dependency on an external Core module/workspace.
- Preserve existing API behavior and Core contracts during the move.
- Extend CI so the self-contained API build is proven with workspace mode disabled.

## Out of Scope

- Temporal workflows or worker implementation.
- Jev integration or speculative decision-provider interfaces.
- Authentication/authorization redesign.
- HTTP behavior cleanup, router replacement, or DTO redesign.
- Broad Core package relocation into a new `internal/core` tree.
- Go module-path rename from `github.com/GARFEX33/garfex-costos-unitarios`.
- Docker/Compose/Makefile operational consolidation beyond Phase 1.

## Constraints

- Do not duplicate or rewrite Core business logic.
- Core domain/application packages must not import HTTP, Temporal, Jev, or external-provider code.
- PostgreSQL remains the source of truth.
- Preserve migrations and runtime/admin role separation.
- Use Git history import rather than filesystem-only copying.
- Keep structural migration separate from behavioral corrections.
- Local project policy forbids `go build` after changes; the API build gate runs in CI.

## Execution Configuration

- Workflow: Organic Driven Development.
- TDD mode: not configured; this is a behavior-preserving structural migration using ordinary regression verification.
- Local verification: `gofmt -l .`, `go vet ./...`, `golangci-lint run ./...`, `GOWORK=off go test ./... -count=1`.
- CI-only verification: `GOWORK=off go test ./... -race -count=1`, `GOWORK=off go build ./...`, and explicit `GOWORK=off go build ./cmd/api`.
- Delivery strategy: `ask-on-risk`.
- Chain strategy: `feature-branch-chain`, selected by the user.
- Forecast: high review-load risk; importing the existing API includes approximately 10,416 tracked Go lines across 60 tracked files, so the authored diff will greatly exceed 400 lines even though most content is pre-existing behavior.
- Planned slices:
  1. API history and HTTP adapter import.
  2. API executable and single-module integration.
  3. Self-contained CI acceptance gates and documentation.

## Tasks

### T1 — Import API history and adapter source

- Status: done.
- Route: delegated writer; multi-file write and preparation triggers apply.
- Outcome:
  - Import API history through an unrelated-history merge parent without importing unrelated API root artifacts.
  - Place API adapter source/tests under `internal/httpapi`.
  - Add only the adapter test dependency required by the root module (`kin-openapi`) and its checksums.
  - Do not redesign handlers or Core contracts.
- Allowed edit surfaces:
  - `internal/httpapi/**`
  - `go.mod`
  - `go.sum`
- Checks:
  - Imported paths have traceable API history through the merge parent.
  - No unrelated API repository root artifacts remain in the backend tree.
  - Structural readback confirms no SQL/repository bypass was introduced.
  - `GOWORK=off go test ./internal/httpapi -count=1` passes.
- Commit evidence: `c276dce74d87c9c2e6a267566ea474773aaff343` (`feat(api): import HTTP adapter history`), with Core parent `6a8c3ed` and API parent `63183bf`.

### T2 — Integrate the API executable into the root module

- Status: done.
- Route: delegated writer; multi-file write trigger applies.
- Outcome:
  - Place the entrypoint and its tests at `cmd/api`.
  - Replace old `garfex-api` module imports with same-module imports.
  - Keep current startup, configuration, lifecycle, and routing behavior.
- Allowed edit surfaces:
  - `cmd/api/**`
- Checks:
  - Exactly one root `go.mod` and `go.sum` own the backend.
  - No committed `go.work` is required.
  - No local `replace` points outside the repository.
  - `GOWORK=off go test ./cmd/api ./internal/httpapi -count=1` passes.
  - `GOWORK=off go test ./... -count=1` passes.
- Commit evidence: `62cc3ffcfbd253942ad1f2f7735ec748a7913773` (`feat(api): integrate API executable`).

### T3 — Add self-contained CI acceptance gates

- Status: in progress; independent pre-commit verification passed, commit creation pending.
- Route: delegated writer; CI configuration plus documentation changes are expected.
- Allowed edit surfaces:
  - `.github/workflows/ci.yml`
  - `README.md`
  - `internal/httpapi/catalog_descriptors_test.go` (focused lint correction required by the imported adapter)
- Outcome:
  - CI proves the backend without workspace mode.
  - CI explicitly builds `./cmd/api`.
  - Repository documentation states the Phase 1 self-contained build contract.
- Checks:
  - `GOWORK=off go list -m -mod=readonly all` resolves without external local replacements.
  - `gofmt -l .` returns no files.
  - `go vet ./...` passes.
  - `golangci-lint run ./...` passes.
  - `GOWORK=off go test ./... -count=1` passes locally.
  - CI includes `GOWORK=off go test ./... -race -count=1`.
  - CI includes `GOWORK=off go build ./cmd/api` or a stronger build covering it.
- Commit evidence: conventional commit `ci: prove self-contained API build`; exact hash is recorded immediately after creation.

### T4 — Verify Phase 1 exit criteria

- Status: pending.
- Route: delegated verification; verification trigger applies.
- Outcome:
  - Demonstrate the repository is a self-contained backend clone.
  - Record every passed, failed, skipped, or CI-only check.
- Acceptance criteria:
  - `cmd/api` and `internal/httpapi` coexist with `resourcecore`, `suppliercore`, `purchasecore`, `cfdicore`, Core domain/application packages, PostgreSQL adapters, and migrations.
  - The API has no Go dependency on a separate GARFEX Core repository.
  - `GOWORK=off go test ./... -count=1` passes.
  - `GOWORK=off go build ./cmd/api` passes in CI.
  - A clean clone needs no sibling GARFEX checkout.
- Commit evidence: pending if verification requires corrective repository changes; otherwise recorded against T3.

## Progress

- 2026-09-21: Completed read-only architecture mapping of both repositories.
- 2026-09-21: User approved beginning consolidation and explicitly accepted the Phase 1 self-contained build gate.
- 2026-09-21: Created branch `feat/consolidate-backend-phase-1`.
- 2026-09-21: User selected `feature-branch-chain` for the oversized consolidation.
- 2026-09-21: Corrected the initial staging-only verification failure by staging the root dependency metadata.
- 2026-09-21: Independent re-verification passed all T1 content, history, boundary, and staging checks.
- 2026-09-21: Created T1 merge commit `c276dce` with both Core and API parents.
- 2026-09-21: Native assessment classified the committed range as high risk and requires a post-commit independent verifier.
- 2026-09-21: Post-commit independent verification passed for T1 commit `c276dce`; no merge-blocking issues remain.
- 2026-09-21: Independent verification passed T2 source equivalence, lifecycle behavior, module isolation, 562 focused tests, and 1,830 full-suite tests.
- 2026-09-21: Created T2 commit `62cc3ff`.
- 2026-09-21: Native assessment was unavailable/schema-incompatible and requires post-commit independent verification.
- 2026-09-21: Post-commit independent verification passed for T2 commit `62cc3ff`; no blockers remain.
- 2026-09-21: T3 CI and README changes were prepared; list, format, vet, tests, and CI inspection passed.
- 2026-09-21: T3 remains partial because `golangci-lint run ./...` found one staticcheck issue in imported `internal/httpapi/catalog_descriptors_test.go`; a focused correction is required before completion.
- 2026-09-21: Corrected imported staticcheck SA1029 with a local named context-key type.
- 2026-09-21: T3 independent verification passed list, format, vet, lint, focused/full tests, CI semantics, documentation accuracy, and changed-path scope.
- Current task: T3 commit creation and post-commit risk-gated verification.

## Verification Evidence

- Baseline Git state: both Core and API repositories were clean on `main` before work began.
- Core baseline: `6a8c3ed` (`Merge pull request #189 from GARFEX33/feat/purchase-line-workbench-core`).
- API import size estimate: approximately 10,416 tracked Go lines and 60 tracked files.
- Writer verification: `GOWORK=off go test ./internal/httpapi -count=1` passed with 548 tests.
- Parent spot-check: the same focused test passed with 548 tests.
- First independent verification: content, history, boundaries, and tests passed; overall result failed because `go.mod` and `go.sum` were not staged for the merge commit.
- Correction: staged `go.mod`, `go.sum`, and this task document.
- Independent re-verification: PASS; 548 adapter tests passed, no unstaged changes remained, all required metadata was staged, the 52-path adapter tree matched `api-import/main`, and no persistence bypass or unwanted API root artifacts were imported.
- Post-commit independent verification: PASS for `c276dce`; exact merge parents, tree equality, dependency metadata, public Core boundaries, and 548 focused tests were confirmed.
- T2 writer verification: PASS; 562 focused tests and 1,830 full-suite tests passed with workspace mode disabled.
- T2 independent verification: PASS; source equivalence, single-module isolation, lifecycle behavior, formatting, and changed-path boundaries were confirmed.
- T2 post-commit independent verification: PASS for `62cc3ff`; 562 focused tests and 1,830 full-suite tests passed, committed source equivalence and module isolation were confirmed.
- T3 initial verification: `go list`, format, vet, 1,830 tests, diff integrity, and CI inspection passed; lint initially failed on one imported staticcheck finding.
- T3 correction verification: PASS; focused test, list, format, vet, lint, 1,830 full-suite tests, diff integrity, and CI inspection passed.
- T3 independent verification: PASS; CI gates, README claims, SA1029 correction, and changed-path scope were confirmed.

## Next Step

Create the verified T3 work-unit commit, record its exact hash, and complete post-commit verification before Phase 1 exit verification.
