# Purchase-line workbench Core contract

Purchase Core now owns the global purchase-line read model, safe manual overrides, and line-centered resolution. Delivery adapters translate this contract; they must not reconstruct status, identity, concurrency, or mapping rules.

## Quick path

1. Read a page with `PurchaseReader.ListPurchaseLinesWorkbench`.
2. Use the row's supplier-product, mapping, and resolution revisions as the mutation snapshot.
3. Call `PurchaseWriter.ResolvePurchaseLine` or `PurchaseWriter.SetResolutionOverride` once.
4. Reread the filtered page after every success, conflict, timeout, or ambiguous commit.

## Global read

`purchasecore.PurchaseLineQuery` accepts:

| Field | Semantics |
|---|---|
| `Limit` | `1..50`; `0` selects `50` |
| `Offset` | `>= 0` |
| `SupplierID` | optional exact supplier |
| `EffectiveStatus` | optional `PENDIENTE`, `VINCULADO`, `SUSPENDIDO`, `NO_APLICA`, or `CONFLICTO` |
| `DateFrom` / `DateTo` | optional inclusive `IssuedAt` bounds |
| `InvoiceText` | case-insensitive series, folio, or CFDI UUID text |
| `SupplierSKU` | case-insensitive XML supplier-SKU text |
| `Description` | case-insensitive line-description text |

`purchasecore.PurchaseLinePage` returns `Rows`, `HasPrevious`, and `HasNext`, ordered by `IssuedAt DESC, LineID DESC`.

Each row carries the required immutable XML facts (line number, description, supplier SKU, SAT product code, quantity/unit, unit price, and amount), document identity, supplier display, nullable commercial SupplierProduct identity, nullable mapping/resource identity, `MappingRevision`, `ResolutionRevision`, stored override, effective status, and effective cause. `SupplierDisplayName` is computed as trade name → legal name → tax identifier → decimal supplier ID. `ResourceDisplayName` is computed set-wise as existing `recursos.display_name` with `ResourceIdentity` fallback; it is not Resource Core's enriched catalog `Describe` output.

## Resolve one line

`purchasecore.ResolvePurchaseLineRequest` requires `LineID`, `ResourceID`, `ExpectedResolutionRevision`, and nonblank `Actor`.

| Observed row | Required snapshot | Commercial SKU |
|---|---|---|
| Existing `SupplierProductID` | matching nonnil `ExpectedSupplierProductID` and nonnil `ExpectedMappingRevision` | must be blank |
| Null `SupplierProductID` | `ExpectedSupplierProductID=nil` and `ExpectedMappingRevision=nil` | stable, nonblank `CommercialSupplierSKU` required |

The command accepts only a locked line whose override is `NONE`, whose effective status is `PENDIENTE`, and whose resolution revision still matches. For a null association, Core derives the supplier from the purchase, creates or reuses `(supplierID, commercialSupplierSKU)`, associates the line, and confirms the mapping in one transaction. Reusing an existing identity is idempotent only when it already maps to the same active Resource; a different target or incompatible mapping state is a conflict.

`PurchaseLine.SupplierSKU` remains immutable XML evidence. The only Resource authority is `SupplierProduct.CurrentMapping`; a line never stores ResourceID.

The result contains the authoritative `PurchaseLine` and `SupplierProduct` snapshots plus `CommercialIdentityDisposition`: `CREATED` when the SupplierProduct was inserted in this transaction, `REUSED` when a preexisting identity received a mapping transition, or `ALREADY_MAPPED` when a preexisting identity already targeted the same active Resource and mapping confirmation was a no-op.

## Manual override

`purchasecore.SetResolutionOverrideRequest` is the sole public override command. It requires:

- `LineID`;
- `Override`: only `NONE`, `NO_APLICA`, or `CONFLICTO`;
- `ExpectedRevision` (`ResolutionRevision`);
- nonblank `Actor` and `Reason`.

Core fixes origin to `manual` and decision time to the current UTC time. A changed decision increments `ResolutionRevision` and appends `purchase_line_resolution_audit` in the same transaction. Repeating the same override with the current revision is an audited-metadata-validated no-op: no new revision and no audit row.

`ResolvePurchaseLine` also checks `ExpectedResolutionRevision`, so changing an override and later returning it to `NONE` invalidates an older resolve snapshot.

## Public error codes

| Code | Suggested HTTP status |
|---|---:|
| `PURCHASE_LINE_NOT_FOUND` | 404 |
| `RESOURCE_NOT_FOUND` | 404 |
| `RESOURCE_INACTIVE` | 422 |
| `COMMERCIAL_SUPPLIER_SKU_REQUIRED` | 422 |
| `COMMERCIAL_SUPPLIER_SKU_FORBIDDEN` | 422 |
| `PURCHASE_LINE_STATE_CONFLICT` | 409 |
| `STALE_RESOLUTION_REVISION` | 409 |
| `STALE_MAPPING_REVISION` | 409 |
| `SUPPLIER_PRODUCT_TARGET_CONFLICT` | 409 |
| `INVALID_MAPPING_TRANSITION` | 409 |
| `INTEGRITY_CONFLICT` | 409 |

Generic `INVALID_ARGUMENT`, `VALIDATION`, `NOT_FOUND`, `CONFLICT`, and `INTERNAL` remain available for existing capabilities. Adapters classify with `purchasecore.Code(err)` and never inspect error strings.

## Out of scope

- canonical enriched Resource `Describe` presentation inside this read model;
- direct `PurchaseLine → Resource` authority;
- automatic suggestions, inferred matching, or ranking;
- blind mutation retries after ambiguous outcomes.
