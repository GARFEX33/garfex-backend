// Package cfdicore reads SAT CFDI 4.0 vouchers (Mexican electronic invoices).
//
// It is a pure, stateless parser: Parse turns XML bytes into an Invoice and
// touches no database, network or file. Callers decide what to do with the
// data, for example prefilling a supplier from Invoice.Issuer.
//
// Contract:
//
//   - Only CFDI 4.0 is accepted. Other versions and non-CFDI documents fail
//     with a typed Error (see ErrorCode); use Code or IsCode to classify.
//   - Elements are matched by namespace URI, never by prefix, so any prefix
//     works. Attribute order does not matter and a UTF-8 BOM is ignored.
//   - Amounts, quantities and rates are exact decimal strings as written in
//     the XML. Nothing is recalculated or verified.
//   - RFCs and UUIDs are trimmed and upper-cased so they compare safely with
//     values stored case-insensitively.
//   - Dates carry no zone in the XML and are returned in UTC.
//   - The digital seal is copied, not verified: this package proves nothing
//     about authenticity.
//
// Not extracted: complements other than TimbreFiscalDigital, and the
// concept-level InformacionAduanera, CuentaPredial and Parte elements.
package cfdicore
