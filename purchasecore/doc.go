// Package purchasecore is the public, consumer-neutral Go contract for the
// GARFEX Purchase and Price History Core. It is a translation boundary,
// not a second business authority: every operation delegates to the
// existing authoritative application service
// (internal/modules/purchases/app.Service) through a module-owned internal
// bridge (internal/bridge/purchasecore.Adapter). This package owns its
// DTOs, validates request shape, and defensively copies every value
// crossing the boundary in both directions; it never aliases an internal
// domain value or exposes a repository, connection pool, or DSN.
//
// # Shipped contract: Reader and Writer
//
// Reader exposes purchase, purchase-line, supplier-product, and
// resource-history reads. Writer exposes CFDI import and the manual
// relation corrections: linking or unlinking a supplier product to a
// Resource Master entry, and overriding one purchase line's LinkStatus.
//
// # Original data is never overwritten
//
// A Purchase and its PurchaseLines carry the exact data taken from the
// imported CFDI. Import is idempotent on the document's fiscal UUID
// (ImportResult.AlreadyExisted) and rejects a same-UUID document whose
// relevant content differs as a CONFLICT, rather than silently
// overwriting what is already registered. Only SupplierProductID and
// LinkStatus on a PurchaseLine change after import, and only through an
// explicit relation correction — never the line's original fields.
//
// # Monetary values
//
// Every quantity, price, and amount is an exact decimal string copied from
// the XML or computed from those strings, never a float, mirroring
// cfdicore's own convention.
//
// # Errors
//
// Every failure returned by this package is an Error carrying one of five
// stable ErrorCode categories (see Code and IsCode): NOT_FOUND,
// VALIDATION, CONFLICT, INVALID_ARGUMENT, and INTERNAL. INVALID_ARGUMENT is
// reserved for request-shape rejection inside this package, before the
// internal service is ever called. Error never formats or retains a
// PostgreSQL message, SQLSTATE, constraint, table, column, or driver type;
// INTERNAL always carries a fixed, generic message regardless of the
// underlying cause.
package purchasecore
