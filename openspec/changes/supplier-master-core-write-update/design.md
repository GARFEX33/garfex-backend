# Design: Graduate `UpdateSupplier` onto the public Supplier Master write contract

## Technical Approach

Repeat the archived Create slice one operation later. `suppliercore` gains `SupplierUpdateRequest` and `Writer.UpdateSupplier` over a two-method `WriteCapabilities`; the existing bridge widens `serviceWriter` by one method and adds `Adapter.UpdateSupplier`, reusing `mapSupplier`/`mapError` byte-for-byte. The Writer validates shape only (`Actor` non-blank, `ID > 0`); all content authority stays in `domain.NewSupplier` via `Supplier.WithDetails`. Zero internal changes, no CAS, no clone helper, no new error branch.

## Architecture Decisions

| # | Decision | Choice | Rejected | Rationale |
|---|---|---|---|---|
| 1 | Validator shape | New `validateSupplierUpdateRequest`; `validateSupplierWriteRequest` untouched | Extend/share Create's validator | It takes `SupplierWriteRequest` — a different Go type, so sharing needs generics or an interface that exists only to be shared ("no abstraction without a real boundary"). Update also adds a rule Create has none of. Matches `resourcecore`'s per-request-type validators. |
| 2 | ID rule | `req.ID <= 0` → `INVALID_ARGUMENT "id must be positive"` | `req.ID == 0` (resourcecore's literal form) | The proposal binds `ID > 0`. A negative id is a shape error, not a business one; `<= 0` keeps the boundary's promise identical to its documentation. Deliberate consequence: it pre-empts internal `validID`'s `VALIDATION` for non-positive ids, the same trade Create made for blank `Actor`. |
| 3 | Inactive suppliers | No active-state gate, no reactivation side effect; documented, not enforced | Reject updates on inactive suppliers | `Service.UpdateSupplier` has no gate and `postgres.UpdateSupplier`'s `SET` omits `active`; `WithDetails` starts from `current` so `Active` survives. A gate would need a read-before-write in the bridge — new business behavior, a `golang-hexagonal` BLOCKER. |
| 4 | Result | `mapSupplier(updated)` | Confirm-read | The internal `RETURNING` row already carries untouched `Active` and refreshed `UpdatedAt`. |
| 5 | Clone | None; add `TestSupplierUpdateRequest_NoReferenceTypedField` | `CloneSupplierUpdateRequest` | All-value struct: Go copies it on call. The guard fails loudly the day a reference field appears. `copy.go` unchanged. |
| 6 | Test files | Extend `writer_test.go`, `external_test.go`, `adapter_test.go` | New `writer_update_test.go` | The fakes (`fakeWriteCapabilities` ×2, `stubService`) are per-package singletons a new file cannot own; at two methods it adds only navigation cost. Revisit at the lifecycle slice or when a fake gains Branch/Contact methods. |

## Data Flow

    consumer ──► Writer.UpdateSupplier(ctx, req)
                   │ Actor blank OR ID <= 0 → INVALID_ARGUMENT (capability never called)
                   ▼
             WriteCapabilities.UpdateSupplier ──► Adapter
                   │ ctx = core.WithActor(ctx, req.Actor)   ── diagnostic only
                   │ req.ID ──► id ; 5 content fields ──► domain.SupplierDetails
                   ▼
             app.Service.UpdateSupplier
                   ├─ GetSupplier ──► ErrSupplierNotFound ──► NOT_FOUND (first write-reachable)
                   ├─ WithDetails ──► NewSupplier ──► VALIDATION   (Active preserved)
                   └─ repo.UpdateSupplier ──► CONFLICT | INTERNAL
                   ▼
             mapSupplier(updated) ──► CloneSupplier ──► public Supplier

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `suppliercore/write_types.go` | Modify | Add `SupplierUpdateRequest{Actor, ID, TradeName, LegalName, TaxIdentifier, Website, Notes}`; document full-replace on the type. |
| `suppliercore/writer.go` | Modify | `UpdateSupplier` on `WriteCapabilities` + `Writer`; `validateSupplierUpdateRequest`. |
| `suppliercore/doc.go` | Modify | Two graduated writes; new `# Update semantics` section; rewritten race paragraph; `NOT_FOUND` now write-reachable. |
| `suppliercore/writer_test.go` | Modify | `fakeWriteCapabilities` gains `updateSupplier`; shape/delegation/clone tests; guard 1 → 2 methods; value-typed-field guard. |
| `suppliercore/external_test.go` | Modify | Package-local `fakeWriteCapabilities` gains the method; `TestExternalConsumer_UpdatesSupplier`. |
| `internal/bridge/suppliercore/adapter.go` | Modify | `serviceWriter` + one method; `Adapter.UpdateSupplier`; package doc says "Create and Update". |
| `internal/bridge/suppliercore/adapter_test.go` | Modify | `stubService` gains `updateSupplier`; mapping/actor, five-code reachability, leakage, `Active`-preserved tests. |
| `suppliercore/copy.go`, `errors.go`, `internal/modules/suppliers/**` | Unchanged | Decisions 3 and 5; five codes already exist. |

## Interfaces / Contracts

```go
type WriteCapabilities interface {
    CreateSupplier(context.Context, SupplierWriteRequest) (Supplier, error)
    UpdateSupplier(context.Context, SupplierUpdateRequest) (Supplier, error)
}

type serviceWriter interface { // internal/bridge/suppliercore
    CreateSupplier(ctx context.Context, details domain.SupplierDetails) (domain.Supplier, error)
    UpdateSupplier(ctx context.Context, id int64, details domain.SupplierDetails) (domain.Supplier, error)
}
```

**GARFEX_STRICT field-completeness gate** — `SupplierUpdateRequest` (7) → `Service.UpdateSupplier(ctx, id, details)`: `ID` → `id`; `TradeName/LegalName/TaxIdentifier/Website/Notes` → `SupplierDetails` 5/5; `Actor` is not a `SupplierDetails` field — it travels via `core.WithActor` and needs the one-line omission comment at the mapping site, identical to `CreateSupplier`. Return direction reuses `mapSupplier` (9/9, already gated).

### `doc.go` replacement text

Shipped-contract paragraph: `"one write operation through Writer: CreateSupplier"` → `"two write operations through Writer: CreateSupplier and UpdateSupplier. Deactivate, Reactivate, and Delete/HardDelete are not exposed for Supplier, and no operation is exposed for Branch or Contact"`.

New section after `# Actor`:

> `# Update semantics`
>
> `UpdateSupplier replaces content; it does not patch it. Every content field of SupplierUpdateRequest is written exactly as supplied, so a field left empty clears the stored value instead of preserving it. A caller intending a partial edit must read the supplier first and resubmit the fields it wants to keep — this package never merges, because choosing what survives a partial write is a business decision it does not own.`
>
> `Update is also lifecycle-neutral and lifecycle-blind: it never changes Active, and it does not refuse a deactivated supplier. Editing an inactive supplier succeeds and leaves it inactive; reactivation is a separate operation this version does not expose.`

Race paragraph — replace the final clause (`"and CreateSupplier inherits the same lost-update race … rather than closing it."`) with:

> `UpdateSupplier exercises that gap directly: the internal service reads the current supplier, rebuilds its details, and writes the result back with no transaction and no compare-and-set, so two concurrent updates resolve last-write-wins and the losing writer's fields disappear without any error. This package reports the risk rather than closing it — closing it would mean inventing a Revision the internal service does not have.`

Errors paragraph — append: `NOT_FOUND is write-reachable for the first time in this version; UpdateSupplier against an unknown id returns it.`

## Testing Strategy

| Layer | What to test | Approach |
|-------|--------------|----------|
| Unit (`suppliercore`) | Blank/whitespace `Actor` and `ID` of `0`/`-1` rejected without touching the capability; success delegates and clones; `WriteCapabilities.NumMethod() == 2` with both names; `Writer` exports no third method; no reference-typed request field | Fake `WriteCapabilities` + `reflect` |
| Unit (bridge) | All 5 details + `ID` reach the stub; `WithActor` set; empty content → VALIDATION; unknown id → NOT_FOUND; `ErrTaxIdentifierConflict` → CONFLICT; raw `PgError`-shaped error → INTERNAL with the fixed message and no SQLSTATE/constraint text; result `Active` reflects the stub's row (never forced true) | Fake `supplierService` with seeded errors |
| External | `suppliercore_test` consumer updates a supplier with no `internal` import | Extend `external_test.go` |
| Integration/E2E | N/A — library-only, nothing wired here | — |

Focused-test discoverability: every new test name must contain `Writer`, `External`, or `Adapter_Update` so the unit's `-run` pattern selects at least one test per touched package.

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable-file classification, or process-integration boundary.

## Migration / Rollout

No migration required. No schema, wiring, or persisted-state change; reverting removes only `UpdateSupplier`.

## Open Questions

None. Proposal question 1 (inactive suppliers) is resolved by decision 3.

Noted, non-blocking: the proposal describes the `NOT_FOUND` edit as reversing a "documented unreachable from Create" claim, but `doc.go` never made that claim (it lives in the spec). The `doc.go` edit is therefore additive, not a correction — the spec-side reversal still applies.
