# Tasks: Graduate `UpdateSupplier` onto the public Supplier Master write contract

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~300–350 (write_types.go +10, writer.go +25, doc.go +30, writer_test.go +90, external_test.go +30, adapter.go +20, adapter_test.go +120) |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending (not needed — single unit) |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: pending
400-line budget risk: Low

Not borderline: ~300–350 leaves 50–100 lines of headroom under 400, and — unlike Create's 422-vs-300–450 overrun — this slice adds no CAS field, no `mapError` branch, and no internal-layer fix. Single work unit recommended; design.md's two-unit split stays available only if the realized `sdd-apply` diff approaches 400.

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | Public `UpdateSupplier` contract + bridge seam, fully tested | PR 1 | `go test ./suppliercore/... ./internal/bridge/suppliercore/... -run 'Writer|External|Adapter_Update' -count=1` | N/A — library-only, no route/CLI/process boundary | Revert the 7 named files; Create and reads stay intact |

## Phase 1: Public Contract (RED→GREEN)

- [x] 1.1 [RED] `writer_test.go`: extend `fakeWriteCapabilities` with `updateSupplier`; add `TestWriter_UpdateSupplier_RejectsBlankActorOrNonPositiveID` (blank/whitespace Actor, ID 0, ID -1). Req: Shape validation Actor+ID.
- [x] 1.2 [RED] `writer_test.go`: add `TestWriter_UpdateSupplier_DelegatesAndClones`, `TestWriter_UpdateSupplier_MutationAfterCallDoesNotLeak`. Req: No error leakage; defensive copying.
- [x] 1.3 [GREEN] `write_types.go`: add `SupplierUpdateRequest{Actor, ID, TradeName, LegalName, TaxIdentifier, Website, Notes}`, full-replace doc comment. Req: Full replace, not a patch.
- [x] 1.4 [GREEN] `writer.go`: add `UpdateSupplier` to `WriteCapabilities`/`Writer`; add `validateSupplierUpdateRequest` (`Actor` blank OR `ID <= 0` → `INVALID_ARGUMENT`, no TrimSpace on content fields). Req: Public write contract Create+Update; shape validation.
- [x] 1.5 [REFACTOR] Confirm 1.1–1.2 green; `validateSupplierWriteRequest`, `copy.go`, `errors.go` untouched.

## Phase 2: Compiled-Surface & Value-Type Guards

- [x] 2.1 `writer_test.go`: extend `TestWriter_NoUngraduatedMethodExported` — `WriteCapabilities.NumMethod() == 2`, allowed set `{CreateSupplier, UpdateSupplier}`. Req: No ungraduated method exported.
- [x] 2.2 `writer_test.go`: add `TestSupplierUpdateRequest_NoReferenceTypedField` reflection guard. Req: No CAS; no Revision field/Actor leak.

## Phase 3: External Consumer Proof

- [x] 3.1 [RED] `external_test.go`: extend package-local `fakeWriteCapabilities` with `UpdateSupplier`; add `TestExternalConsumer_UpdatesSupplier` (no `internal` import). Req: External package updates a supplier.
- [x] 3.2 [GREEN] Confirm 3.1 compiles/passes once Phase 1 lands.

## Phase 4: `doc.go` Rewrite

- [x] 4.1 Replace shipped-contract sentence with the two-operation text (design.md verbatim).
- [x] 4.2 Insert `# Update semantics` section after `# Actor` — full-replace + lifecycle-blind clauses, verbatim from design.md.
- [x] 4.3 Replace race paragraph's final clause with Update's read-rebuild-write sentence, verbatim from design.md.
- [x] 4.4 Append the `NOT_FOUND` write-reachable sentence to the errors paragraph, verbatim from design.md.

## Phase 5: Bridge Seam

- [x] 5.1 [RED] `adapter_test.go`: extend `stubService` with `updateSupplier`; add `TestAdapter_UpdateSupplier_FieldCompleteness` (5 `SupplierDetails` fields + `ID`, `GARFEX_STRICT`).
- [x] 5.2 [RED] `adapter_test.go`: add `TestAdapter_UpdateSupplier_AllFiveErrorCategoriesReachable` (`INVALID_ARGUMENT`, `VALIDATION`, `NOT_FOUND`, `CONFLICT`, `INTERNAL`), `TestAdapter_UpdateSupplier_InactiveSupplierStaysInactive`, `TestAdapter_UpdateSupplier_ActiveSupplierNeverFlipped`, `TestAdapter_UpdateSupplier_MutationAfterCallDoesNotLeak`.
- [x] 5.3 [GREEN] `adapter.go`: widen `serviceWriter` with `UpdateSupplier(ctx, id int64, details domain.SupplierDetails) (domain.Supplier, error)`; add `Adapter.UpdateSupplier` (translate via `core.WithActor`, reuse `mapSupplier`/`mapError` unmodified).
- [x] 5.4 [REFACTOR] Confirm 5.1–5.2 green; zero new mapping/error branches; `internal/modules/suppliers/**` untouched.

## Phase 6: Full-Suite Verification

- [x] 6.1 `go test ./suppliercore/... ./internal/bridge/suppliercore/... -run 'Writer|External|Adapter_Update' -count=1`.
- [x] 6.2 `go test ./... -count=1`; `gofmt -l .`; `go vet ./...`; `golangci-lint run ./...`; confirm no diff outside the 7 named files.
