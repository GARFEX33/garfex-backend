# SupplierProduct mapping PR chain

Issue: #183
Strategy: feature-branch chain
Tracker branch: `feat/supplier-product-mapping-tracker`

## Review order

1. `feat/supplier-product-mapping-01-domain`
   - Domain mapping transitions and effective line resolution.
   - Review budget: 1,071 changed lines / 400.
   - Exception rationale: splitting the domain model from its behavior tests and compile-preserving consumers leaves a non-compiling intermediate state.
2. `feat/supplier-product-mapping-02-persistence`
   - Migration, atomic PostgreSQL persistence, application use cases, public Purchase Core contracts, and bridge.
   - Review budget: 2,447 changed lines / 400.
   - Exception rationale: persistence and semantic public writes must land together to avoid an unsafe legacy bypass or uncompilable intermediate contract.
3. `docs/supplier-product-mapping-03-composition`
   - Root composition assertion and public documentation.
   - Review budget: 62 changed lines / 400.

## Merge order

Merge the chain from the leaf back to the tracker:

1. PR 3 into PR 2's branch.
2. PR 2 into PR 1's branch.
3. PR 1 into the tracker branch.
4. Tracker PR into `main` only after every child is green and reviewed.

The tracker is draft/no-merge until all child work units are integrated.
