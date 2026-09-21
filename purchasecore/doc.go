// Package purchasecore is the public, consumer-neutral Go contract for the
// GARFEX Purchase and Price History Core. It exposes fiscal purchase facts,
// current SupplierProduct mapping knowledge, derived line resolution, and
// semantic mapping/override commands through one bridge-backed boundary.
//
// SupplierProduct.CurrentMapping is the sole confirmed Resource authority;
// PurchaseLine stores only its SupplierProduct association, never ResourceID.
// Mapping revisions and append-only audit entries are changed only by the
// semantic mapping commands. PurchaseLine preserves XML facts and stores an
// audited ResolutionOverride plus ResolutionRevision; EffectiveStatus and
// EffectiveCause are derived from that override, current mapping knowledge,
// and Resource.Active.
package purchasecore
