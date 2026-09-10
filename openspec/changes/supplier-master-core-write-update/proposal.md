# Graduate `UpdateSupplier` onto the public Supplier Master write contract

| Session setting | Value |
| --- | --- |
| execution_mode | auto |
| artifact_store | hybrid |
| delivery_strategy | ask-on-risk |
| review_budget_lines | 400 |
| chain_strategy | stacked-to-main |

## Intent

`suppliercore` exports exactly one write method, `CreateSupplier` (archived `supplier-master-core-write`, PRs #173/#174). An external consumer can create a supplier but cannot correct one: `internal/modules/suppliers/app.Service.UpdateSupplier` already exists, is correctly wired end-to-end, and is unreachable from outside the module — a `golang-hexagonal` `MISSING PUBLIC CAPABILITY`, resolved by extending the public contract rather than reconstructing behavior anywhere. This is the second Writer slice of the per-operation graduation series, mirroring resourcecore (`write` → `-update` → `-lifecycle`).

## User outcome

An external Go consumer holding a `suppliercore.Writer` submits a `SupplierUpdateRequest` and receives the persisted `suppliercore.Supplier`, or a stable public `Error` — `INVALID_ARGUMENT`, `VALIDATION`, `NOT_FOUND`, `CONFLICT`, `INTERNAL` — with no PostgreSQL detail. No other write operation becomes exported or discoverable.

## Binding decisions (settled — not reopened)

| Area | Decision |
| --- | --- |
| Slice boundary | Supplier Update only. `WriteCapabilities` grows 1 → 2 methods (`CreateSupplier`, `UpdateSupplier`). |
| Request shape | `SupplierUpdateRequest{Actor, ID, TradeName, LegalName, TaxIdentifier, Website, Notes}` — Create's five content fields plus `ID`, because `Service.UpdateSupplier(ctx, id, details)` takes `id` separately. |
| Semantics | **Full replace, not patch.** `Supplier.WithDetails` rebuilds via `NewSupplier(details)` and overwrites all five fields; no merge logic exists. An omitted field genuinely clears it. Must be stated in `doc.go`. |
| Shape validation | `Actor` non-blank **and** `ID > 0` only. Content emptiness stays `domain.NewSupplier`'s sole authority: `Actor`+`ID` set with zero content returns `VALIDATION`, not boundary `INVALID_ARGUMENT`. |
| CAS / Revision | Still absent. Not added anywhere in this series. |
| Errors | Zero new codes. Update is the first operation reaching all five; `NOT_FOUND` flips from "documented unreachable from Create" to genuinely write-reachable (`GetSupplier` → `ErrSupplierNotFound` → existing `ErrNotFound` branch). |
| Internal layer | **No changes.** `Service.UpdateSupplier`, `WithDetails`, and `mapWriteError` are already correct — unlike resourcecore's Update, which needed a 2-line CONFLICT fix. |
| Bridge | Widen the existing `serviceWriter` seam by one method; add `Adapter.UpdateSupplier` reusing `mapSupplier`/`mapError` **unmodified**. Zero new mapping or error branches. |
| `doc.go` race language | Rewrite, in scope. The current forward-pointing sentence ("CreateSupplier inherits the same lost-update race…") becomes misleading once Update is the method mechanically exercising it: `GetSupplier` → `WithDetails` → `repo.UpdateSupplier`, no transaction, no CAS, silent last-write-wins. Documentation correction only — fixing the race means adding CAS, which stays out of scope. |
| Clone helper | None. `SupplierUpdateRequest` is all value-typed; a `TestSupplierUpdateRequest_NoReferenceTypedField` reflection guard proves no aliasing. `copy.go` unchanged. |
| HardDelete | Permanently absent. No internal capability exists to wrap. |

## Capabilities

### New Capabilities

- None.

### Modified Capabilities

- `supplier-master-core`: extend "Public write contract — Supplier Create only" to Create **and** Update; extend "Shape validation is Actor-only…" to Actor + positive `ID` on the Update path; extend "Create-reachable error taxonomy…" so `NOT_FOUND` is write-reachable; restate the no-CAS requirement in terms of Update's directly-exercisable lost-update race; raise the compiled-surface guard from one graduated method to two.

## Scope

### In scope

- `suppliercore/write_types.go`: `SupplierUpdateRequest` (additive; `SupplierWriteRequest` untouched).
- `suppliercore/writer.go`: `UpdateSupplier` on `WriteCapabilities` and `Writer`; `validateSupplierUpdateRequest` (shape only).
- `internal/bridge/suppliercore/adapter.go`: one-method `serviceWriter` widening + `Adapter.UpdateSupplier`.
- Reachability coverage for all five error codes from Update, with no driver leakage.
- External-package Update proof with no `internal` import; reflection guard updated to two methods.
- `doc.go`: graduated-operation count, full-replace semantics, `NOT_FOUND` reversal, rewritten race paragraph.

### Non-goals

- Branch and Contact writes (later slices).
- `DeactivateSupplier` / `ReactivateSupplier` (later slice).
- Any hard delete, ever.
- Adding `Revision`/CAS, or fixing the internal lost-update race.
- Partial-update (PATCH) request shape — the bridge would have to read-merge-write, which is new business behavior it must never own.
- New error codes, new services, internal business-rule changes, or any wiring/composition root.

## Affected areas

| Area | Impact | Description |
| --- | --- | --- |
| `suppliercore/write_types.go` | Modified | Add `SupplierUpdateRequest`. |
| `suppliercore/writer.go` | Modified | Second write method + shape validator. |
| `suppliercore/doc.go` | Modified | Operation count, full-replace, `NOT_FOUND`, race rewrite. |
| `suppliercore/copy.go`, `errors.go` | Unchanged | Value-typed request; five codes already exist. |
| `internal/bridge/suppliercore/adapter.go` | Modified | `serviceWriter` + `Adapter.UpdateSupplier`; mapping reused as-is. |
| `internal/modules/suppliers/**` | Unchanged | Authority already correct; no internal fix needed. |
| Tests | Modified | `writer_test.go`, `external_test.go`, `bridge/suppliercore/adapter_test.go`. |

## Architecture impact

Dependency direction unchanged: consumer → `suppliercore` → `internal/bridge/suppliercore` → `app.Service` → domain → PostgreSQL. The `Writer` stays a translation and shape-validation boundary, never an authority. `suppliercore` keeps zero `internal` imports.

## Open questions

Resolved in exploration with code evidence (six investigation questions; not reopened here).

Genuinely new, surfaced while writing this proposal — for the user or `sdd-design` to confirm:

1. **Updating an inactive supplier.** `Service.UpdateSupplier` has no active-state gate, and `postgres.UpdateSupplier`'s `SET` clause omits the `active` column, so a deactivated supplier's details can be freely edited and its `Active` flag is never changed by Update. Proposed default: mirror internal behavior exactly (no boundary gate, no reactivation side effect) and state both facts in `doc.go`, so a consumer neither assumes deactivated records are read-only nor expects Update to reactivate. Adding an active-state gate would be new business behavior and is out of scope.

## Risks and mitigations

| Risk | Likelihood | Mitigation |
| --- | --- | --- |
| Consumer assumes partial-update semantics and silently clears fields | High | State full-replace explicitly in `doc.go` and in the spec's Update requirement; test that a blank optional field clears it. |
| Stale Create-phrased race language misleads Update callers | Med | `doc.go` rewrite is an in-scope deliverable, not a follow-up. |
| Bridge re-implements merge or validation logic | Med | Translate-and-delegate only; any conditional business decision is a BLOCKER. |
| Silent field loss on the Update path | Med | `GARFEX_STRICT` field-completeness gate: five content fields + `ID` mapped field by field. |
| `NOT_FOUND` reversal left undocumented | Low | Explicit reachability test plus `doc.go` and spec updates. |
| Budget overrun | Low | Estimate below is well inside 400; `ask-on-risk` still decides before apply. |

## Rollback boundaries

- Reverting removes only `UpdateSupplier`; Create and the entire read contract stay intact and tested.
- No migration, schema, wiring, or internal-service rollback — nothing new is persisted or composed.
- The `doc.go` race rewrite may stay on revert: it describes pre-existing internal behavior accurately either way.
- No rollback may weaken error sanitization or the compiled-surface guard to restore green.

## Success criteria

- [ ] An external Go package updates a supplier through `suppliercore` with no `internal` import.
- [ ] `suppliercore` compiles with exactly two write methods; the reflection guard fails on any third or stubbed method.
- [ ] All five `domain.SupplierDetails` fields plus `ID` reach the internal call, or carry a one-line omission rationale.
- [ ] `INVALID_ARGUMENT`, `VALIDATION`, `NOT_FOUND`, `CONFLICT`, and `INTERNAL` are each proven reachable from Update.
- [ ] Zero changes under `internal/modules/suppliers/`; zero new `mapError` branches.
- [ ] No public error string, type, or unwrap chain exposes pgx, SQLSTATE, constraint, table, or column detail.
- [ ] `doc.go` states both graduated operations, full-replace semantics, the write-reachable `NOT_FOUND`, and Update's direct lost-update race.
- [ ] `gofmt -l .`, `go vet ./...`, `golangci-lint run ./...`, `go test ./... -count=1` pass; CI-only race/build reported separately.
- [ ] Strict red-green-refactor evidence preserved.

## Delivery forecast

Estimated **250–350 authored lines** — smaller than Create's actual 422: no CAS field, no new `mapError` branch, no internal-layer fix, and validators/guards are copy-adapted rather than invented. This fits a single unit inside the 400-line budget.

Recommendation: `sdd-tasks` should still forecast the same **two-unit stacked chain Create used** — (1) public contract (`write_types.go`, `writer.go`, `doc.go`, guard/shape tests), then (2) bridge seam + error-reachability coverage + external proof — and let `ask-on-risk` collapse it to one unit if the realized estimate holds under 400. Rationale: the estimate is a projection off a precedent that overran its own 300–450 forecast, the two units have clean independent boundaries and rollback, and the stacked-to-main chain is already an established, low-friction precedent in this series.
