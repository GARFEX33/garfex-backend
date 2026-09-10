# Exploration: `supplier-master-core-write-update` — graduate `UpdateSupplier`

## Current State

`suppliercore` (archived `supplier-master-core-write`, PRs #173/#174) exposes exactly one write method, `CreateSupplier`, via `Writer`/`WriteCapabilities` → `internal/bridge/suppliercore.Adapter` → `internal/modules/suppliers/app.Service.CreateSupplier` → `domain.NewSupplier`. The internal authority for Update already exists and is fully wired, unused by any public contract yet:

- `internal/modules/suppliers/app/supplier.go:31-42` — `Service.UpdateSupplier(ctx, id int64, details domain.SupplierDetails) (domain.Supplier, error)`: `GetSupplier(ctx, id)` → `current.WithDetails(details)` → `repo.UpdateSupplier(ctx, next)`, wrapped with `wrap("update supplier", err)`.
- `internal/modules/suppliers/domain/supplier.go:39-47` — `Supplier.WithDetails` calls `NewSupplier(details)` fresh and overwrites `TradeName, LegalName, TaxIdentifier, Website, Notes` wholesale — **full replace, not a patch**. The exact same "trade name, legal name, or tax identifier required" gate in `NewSupplier` (line 33-35) runs on every `WithDetails` call, so it applies identically on Update.
- `internal/modules/suppliers/postgres/supplier.go:70-80` — `UpdateSupplier` SQL sets all five columns unconditionally (`SET trade_name=$2, legal_name=$3, tax_identifier=NULLIF($4,''), website=$5, notes=$6 WHERE id=$1`), `pgx.ErrNoRows` → `domain.ErrSupplierNotFound`, any error → `mapWriteError` (unique-violation on `suppliers_tax_identifier_key` → `domain.ErrTaxIdentifierConflict`).
- `internal/modules/suppliers/domain/errors.go:8-16` — `ErrSupplierNotFound = fmt.Errorf("%w: supplier", ErrNotFound)`, `ErrTaxIdentifierConflict = fmt.Errorf("%w: tax identifier", ErrConflict)` — both already `errors.Is`-compatible with the same sentinels Create uses.
- `internal/bridge/suppliercore/adapter.go:41-45,317-330` — `serviceWriter` currently declares only `CreateSupplier`; `mapError`'s four branches (`ErrNotFound→NotFound`, `ErrBranchOwnership→Validation`, `ErrValidation→Validation`, `ErrConflict→Conflict`, default→`Internal`) are already generic and need zero new branches for Update.
- `suppliercore/writer.go`, `write_types.go`, `doc.go`, `copy.go` — no CAS/Revision field exists anywhere in this package (binding decision, confirmed still true).

The precedent this mirrors, `resourcecore`'s own Update slice (archived `2026-08-20-resource-master-core-write-update`), added `CatalogUpdateRequest`/`ResourceUpdateRequest` with `ID` + `ExpectedRevision` (CAS) fields, full-replace semantics, and needed a genuine two-line internal CONFLICT-wiring fix because two postgres sentinels weren't yet aliased to `domain.ErrRevisionConflict`. Neither of those gaps exists here: `suppliercore` has no CAS to add, and `ErrTaxIdentifierConflict`/`ErrSupplierNotFound` are already correctly wired to `ErrConflict`/`ErrNotFound` today — this slice needs **no internal-layer fix**, only additive public-contract and bridge work.

## Answers to the six specific investigation questions

1. **Identifying the target record**: `Service.UpdateSupplier(ctx, id int64, details domain.SupplierDetails)` takes `id` as a separate parameter, not embedded in `SupplierDetails`. `SupplierUpdateRequest` must carry `ID int64` alongside the five `SupplierDetails`-mirroring fields, exactly the resourcecore shape minus `ExpectedRevision`.

2. **Partial vs full-replace**: Confirmed full-replace. `WithDetails` builds a brand-new `Supplier` via `NewSupplier(details)` and copies exactly five fields (`TradeName, LegalName, TaxIdentifier, Website, Notes`) onto the current record — there is no per-field "only update what's set" merge logic anywhere in the domain or postgres layer. `SupplierUpdateRequest` should therefore carry the same five content fields as `SupplierWriteRequest`, all always sent, with no partial-update markers (mirrors resourcecore's own "full-replacement write" Update decision).

3. **Empty-content validation on Update**: Same rule as Create, same authority. `WithDetails` → `NewSupplier(details)` re-checks `TradeName == "" && LegalName == "" && TaxIdentifier == ""` unconditionally. Updating a supplier down to zero identifiers returns `domain.ErrValidation` (`NewValidationError("supplier", ...)`) before any repository call — surfaces as public `VALIDATION`, identical to Create's reachability. No boundary-level "at least one identifier" duplication needed in the `Writer`; shape validation stays Actor-plus-ID only.

4. **Not-found handling**: `Service.UpdateSupplier`'s first step is `s.GetSupplier(ctx, id)` (which itself wraps `repo.GetSupplier` → `wrap("get supplier", ...)`). For a nonexistent ID, `repo.GetSupplier` returns `fmt.Errorf("%w: id %d", domain.ErrSupplierNotFound, id)`; `ErrSupplierNotFound` already wraps `ErrNotFound`. The chain remains `errors.Is`-transparent through both `wrap` calls, so `mapError`'s existing first case (`errors.Is(err, domain.ErrNotFound)`) catches it cleanly → public `NotFound`. No gap, no new `mapError` branch needed — this is the case Create's design already anticipated ("`NOT_FOUND` is unreachable from Create and stays read-only-proven"; Update is where it finally becomes write-reachable too).

5. **Tax identifier conflict on Update**: Confirmed reachable the same way as Create. `postgres/supplier.go`'s `UpdateSupplier` SQL includes `tax_identifier=NULLIF($4,'')`; a collision triggers PostgreSQL's `23505` on `suppliers_tax_identifier_key`, and `mapWriteError` (`internal/modules/suppliers/postgres/repository.go:56-81`) maps that exact constraint name to `domain.ErrTaxIdentifierConflict` — which wraps `ErrConflict` — reaching `mapError`'s existing `ErrConflict → Conflict` branch. No new mapping code required.

6. **Lost-update race on Update — genuinely new, needs an explicit doc.go rewrite, not just reuse**: `Service.UpdateSupplier` performs an unguarded read-then-write (`GetSupplier` then `repo.UpdateSupplier`) with no transaction and no CAS field anywhere to detect a stale read. Two concurrent `UpdateSupplier` calls on the same ID will silently last-write-wins, discarding whichever change lost the race — with zero error, zero signal. The current `doc.go` sentence — *"CreateSupplier inherits the same lost-update race the internal UpdateSupplier/Set*Active paths already have"* — was written from Create's perspective (Create has no prior state to race against; it only referenced this as a forward pointer). Once this slice ships, `UpdateSupplier` *is* the internal path the sentence points at, and the race becomes directly, mechanically exercisable through the public contract for the first time. **Recommendation: this needs a doc.go rewrite** (not just "the existing language already covers it"), replacing the forward-pointing phrasing with a direct statement: *"UpdateSupplier performs no optimistic-concurrency check; two concurrent updates to the same supplier ID silently last-write-wins, with no error and no signal to either caller."* This is a documentation-only change (no behavior fix — fixing it would mean adding CAS, which is explicitly out of scope per the binding decisions), but leaving the old wording as-is would be actively misleading once Update ships.

## Affected Areas

| File | Impact |
| --- | --- |
| `suppliercore/write_types.go` | Add `SupplierUpdateRequest{Actor, ID, TradeName, LegalName, TaxIdentifier, Website, Notes}` — additive, `SupplierWriteRequest` untouched. |
| `suppliercore/writer.go` | Add `UpdateSupplier` to `WriteCapabilities` (2nd method) and `Writer`; add `validateSupplierUpdateRequest` (Actor non-blank + `ID > 0` → `INVALID_ARGUMENT`; content stays domain-owned). |
| `suppliercore/writer_test.go` | Extend `fakeWriteCapabilities` with `updateSupplier`; add nil-guard/shape/delegation/clone tests; reflection guard count moves from 1→2 method(s), allowed-method set grows. |
| `suppliercore/external_test.go` | Add an external-package Update proof, no `internal` import. |
| `suppliercore/doc.go` | Update "read plus Supplier Create" → "read plus Supplier Create and Update"; rewrite the lost-update-race paragraph per point 6 above; note `NOT_FOUND` becomes write-reachable via Update. |
| `internal/bridge/suppliercore/adapter.go` | Widen `serviceWriter` with `UpdateSupplier(ctx, id int64, details domain.SupplierDetails) (domain.Supplier, error)`; add `Adapter.UpdateSupplier` translating `SupplierUpdateRequest` → `(id, domain.SupplierDetails)`, reusing `mapSupplier`/`mapError` unmodified. |
| `internal/bridge/suppliercore/adapter_test.go` | Extend `stubService` with `updateSupplier`; add field-completeness (5/5 `SupplierDetails` + `ID`), `NOT_FOUND`/`VALIDATION`/`CONFLICT`/`INTERNAL` reachability, and no-leakage tests for Update. |
| `internal/modules/suppliers/**` | Unchanged — authority (`Service.UpdateSupplier`, `WithDetails`, `mapWriteError`) already exists and is already correctly wired; no internal fix needed (unlike resourcecore's Update, which needed a 2-line CONFLICT-wiring fix). |
| `suppliercore/copy.go`, `errors.go` | Unchanged — `SupplierUpdateRequest` is all value-typed (mirrors the Create decision to omit a clone helper; confirm with the same `TestSupplierUpdateRequest_NoReferenceTypedField` reflection guard pattern rather than adding an unnecessary `CloneSupplierUpdateRequest`). |

## Approaches

1. **One slice, mirrors Create's shape 1:1 minus CAS (recommended, and effectively the only real option)** — `SupplierUpdateRequest{Actor, ID, TradeName, LegalName, TaxIdentifier, Website, Notes}`, full-replace semantics, `WriteCapabilities` grows to 2 methods, bridge widens `serviceWriter` by one method, zero new error codes, zero internal-layer changes.
   - Pros: Directly dictated by the existing internal signature (`Service.UpdateSupplier(ctx, id, details)`) and by all six binding decisions already settled for the series; fully mirrors both precedents (Create's own package conventions, resourcecore's Update shape minus CAS); no internal fix needed (unlike resourcecore Update); smallest possible diff given the existing authority.
   - Cons: None structural — the doc.go race-language rewrite (point 6) is a small but real content debt to pay in this slice.
   - Effort: Low. Estimated 250-350 authored lines (smaller than Create's actual 422, since no CAS field, no new mapError branch, and validators/clone-guard patterns are copy-adapted from Create rather than invented). Fits comfortably in the 400-line budget as a single unit, or splits cleanly into the same two-unit chain Create used (public contract, then bridge) if `ask-on-risk` calls for it.

2. **Partial-update (PATCH) request shape, only sending changed fields** — rejected. `WithDetails`/`NewSupplier` have no merge semantics; supporting this at the boundary would require the bridge to read-merge-write itself, which is new business behavior the internal service doesn't own — a direct violation of "bridge translates only, never re-implements business rules." No real alternative here.

3. **Add a lightweight non-persisted CAS-like guard (e.g., timestamp check) to blunt the lost-update race** — rejected outright by the already-settled binding decision ("No CAS/Revision anywhere in `suppliercore`... the public contract inherits the internal service's existing lost-update race, documented rather than closed"). Not re-litigated; listed only to confirm it was considered and is out of scope.

This is a mechanical, low-ambiguity slice — approaches 2 and 3 are not genuine forks, they're both foreclosed by either the existing internal authority's shape or an already-settled series-wide decision. Approach 1 is the only viable path.

## Recommendation

Approach 1. Follow `sdd-propose` with: `SupplierUpdateRequest{Actor, ID, TradeName, LegalName, TaxIdentifier, Website, Notes}`; `Writer.UpdateSupplier` shape-validates `Actor` non-blank and `ID > 0` only (content stays `domain.NewSupplier`'s authority, exactly like Create); bridge widens `serviceWriter` by one method and adds `Adapter.UpdateSupplier` with zero new mapping/error-mapping code; `doc.go` gets both an operation-count update and the lost-update-race paragraph rewrite from point 6. No internal-layer changes required. Likely fits a single ≤400-line unit but should still forecast a two-unit stacked chain (public contract, then bridge) in `sdd-tasks` given `ask-on-risk`, matching Create's own successful stacked-delivery precedent.

## Risks

- Doc.go's lost-update-race language, if left as Create-phrased ("CreateSupplier inherits..."), becomes misleading once `UpdateSupplier` is the method actually exercising the race — must be rewritten, not just left in place (see point 6).
- `NOT_FOUND` shifts from "documented unreachable from Create" to genuinely write-reachable via Update; the doc.go and readiness evidence must reflect this reversal (mirrors resourcecore Update's own analogous NOT_FOUND reversal for catalog Update).
- Full-replace semantics mean a caller who omits a field (e.g. sends blank `Website`) genuinely clears it — this is existing internal behavior, not new risk introduced by this slice, but should be stated explicitly in `doc.go` so a consumer doesn't assume partial-update semantics.
- Field-completeness discipline: `SupplierUpdateRequest`'s five content fields must map 1:1 to `domain.SupplierDetails`, same GARFEX_STRICT gate Create already passed — low risk given the fields are copy-adapted from an already-proven mapping.
- Review-budget risk is Low given the actual Create precedent (422 lines total, well-contained) and the absence of any internal-layer fix this time.

## Ready for Proposal

Yes. All six investigation questions are resolved with concrete code evidence; no internal-layer defect or gap was found (unlike resourcecore's Update, which needed a genuine CONFLICT-wiring fix — this slice needs none). The only item to flag explicitly to the user before `sdd-propose` commits: confirm the doc.go lost-update-race paragraph should be rewritten (not merely left as-is) to describe `UpdateSupplier`'s directly-exercisable race — this is a required content correction, not a re-litigation of the no-CAS decision itself.
