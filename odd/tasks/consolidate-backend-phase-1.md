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
- Commit evidence: merge commit with Core parent `6a8c3ed` and API parent `63183bf`; exact merge commit hash is recorded immediately after creation.

### T2 — Integrate the API executable into the root module

- Status: pending.
- Route: delegated writer; multi-file write trigger applies.
- Outcome:
  - Place the entrypoint at `cmd/api`.
  - Replace old `garfex-api` module imports with same-module imports.
  - Keep current startup, configuration, lifecycle, and routing behavior.
- Checks:
  - Exactly one root `go.mod` and `go.sum` own the backend.
  - No committed `go.work` is required.
  - No local `replace` points outside the repository.
  - `GOWORK=off go test ./... -count=1` passes.
- Commit evidence: pending.

### T3 — Add self-contained CI acceptance gates

- Status: pending.
- Route: delegated writer; CI configuration plus documentation changes are expected.
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
- Commit evidence: pending.

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
- Current task: T1 merge commit creation, followed by T2.

## Verification Evidence

- Baseline Git state: both Core and API repositories were clean on `main` before work began.
- Core baseline: `6a8c3ed` (`Merge pull request #189 from GARFEX33/feat/purchase-line-workbench-core`).
- API import size estimate: approximately 10,416 tracked Go lines and 60 tracked files.
- Writer verification: `GOWORK=off go test ./internal/httpapi -count=1` passed with 548 tests.
- Parent spot-check: the same focused test passed with 548 tests.
- First independent verification: content, history, boundaries, and tests passed; overall result failed because `go.mod` and `go.sum` were not staged for the merge commit.
- Correction: staged `go.mod`, `go.sum`, and this task document.
- Independent re-verification: PASS; 548 adapter tests passed, no unstaged changes remained, all required metadata was staged, the 52-path adapter tree matched `api-import/main`, and no persistence bypass or unwanted API root artifacts were imported.

## Next Step

Create the verified T1 merge commit, record its exact hash, and begin T2 executable integration.
