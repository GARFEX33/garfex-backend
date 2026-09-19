# Feature: cfdi-xml-parser

## Objective
Read SAT CFDI 4.0 invoice XML files and extract all their data, exposed through an
HTTP endpoint so a UI can offer "load supplier from XML" (first consumer: Suppliers screen).

## Problem / why
Supplier invoices arrive as CFDI XML. Users retype Emisor data by hand. Parsing the XML gives
the supplier draft (RFC, legal name, regimen) and, later, the invoice concepts for other flows.

## Scope
- Core: new pure public package `cfdicore` (stdlib `encoding/xml`, no DB, no new deps).
- garfex-api: `POST /v1/cfdi/parse` (multipart `file` or raw XML body) returning parsed data
  plus a `supplierDraft` built from the Emisor. Parse only; nothing is persisted.
- Out of scope: creating/matching suppliers, matching concepts to resources, RFC checksum
  validation, SAT catalog labels, complementos other than TimbreFiscalDigital,
  InformacionAduanera / CuentaPredial / Parte, CFDI 3.3, signature verification.

## Constraints / decisions
- Match by namespace URI (`http://www.sat.gob.mx/cfd/4`), never by prefix; tolerate UTF-8 BOM
  and any attribute order.
- Money/quantities kept as exact decimal strings (no float64).
- RFC trimmed + uppercased; UUID uppercased (samples mix cases; supplier unique index is case-insensitive).
- Errors: `cfdicore.Error` with stable codes, same shape as suppliercore.
- Supplier mapping: Rfc -> taxIdentifier, Nombre -> legalName; RegimenFiscal exposed but not persistable
  (no supplier field). tradeName is left for the user.
- Body limit stays at the API's 1 MiB; samples are 6-13 KB.
- Commits: conventional, NO Co-Authored-By trailer (user global rule + AGENTS.md override the harness reminder).
- Fixtures: synthetic testdata committed; docs/ejemplo xlm (real RFCs, untracked) only read by an
  optional test that skips when absent.

## Checks
- TDD: enabled (project setting "Strict TDD Mode"), runner `go test ./...` (Core), `go test ./internal/httpapi/...` (API).
- Also: gofmt, go vet.
- Route: inline (project memory: delegated writers hit rate limits on this repo). Trigger evidence:
  mapping already delegated (Explore), writes done directly.
- Delivery forecast: ~650 authored lines total across 2 repos, ~350 per repo. Strategy: ask-on-risk (each repo under ~400).
- RDD: not enabled by user -> no native review.

## Tasks
- [x] T1 cfdicore: types, errors, Parse (BOM, ns-by-URI, validation) + tests (synthetic fixtures + optional real samples) — Core repo. Commit f6c1805 (771 lines, ~60% tests/fixtures: over the 400 heuristic because tests+types+fixture ship together). RED observed (undefined Parse), GREEN 16 tests, `-race`, vet, gofmt, golangci-lint 0 issues. Real samples: 4/4 parse, exact accounting invariants hold. Route: inline. RDD off.
- [x] T2 garfex-api: POST /v1/cfdi/parse handler, supplierDraft mapping, OpenAPI, README, tests — API repo (branch feat/cfdi-parse-endpoint). Commit 3da6a46 (808 lines, ~70% tests+spec). RED observed (14 tests 404), GREEN; full API suite 401+ pass; real samples validate against OpenAPI schema. Only CFDI hunks staged: router.go/openapi.yaml also hold uncommitted attribute-order work from another feature, left untouched. Route: inline.
- [x] T3 Core README mention + final verification. Commit 56213db. `go test -race ./...` Core: 16 packages ok, 0 FAIL; API `go test ./...` ok; gofmt/vet clean on new files (gofmt flags pre-existing catalog_descriptors.go in API, not touched).

### Phase 2: supplier preview (user decision: parser is shared, each use case has its own endpoint that receives the file)
- [x] T4 (commit 2f18664, 273 lines; RED observed on 4 layers, GREEN, clean-worktree `go test -race ./...` 15 pkgs ok; SQL only shape-tested: no real-Postgres run, needs an isolated Docker DB that the user must authorize) Core suppliercore: exact lookup `GetSupplierByTaxIdentifier` (trim+upper, includes INACTIVE, NotFound when absent) across suppliercore/bridge/app/domain/postgres + tests. Route: inline (project memory: delegated writers rate-limited); trigger evidence: 5 layers mapped by hook context + direct reads.
- [x] T5 (commit 93bb860, RED observed 404s, GREEN) garfex-api: `POST /v1/suppliers/from-cfdi/preview` (multipart/raw file) -> `{draft, existing|null}`; reuses cfdicore + T4; read-only; OpenAPI/README/tests. Creation stays `POST /v1/suppliers`.
- [x] T6 Final verification. INCIDENT: commit 3da6a46 shipped an invalid openapi.yaml (filtered-hunk staging cut a components block); earlier verification ran on a dirty working tree. Fixed in 9ea1e35 (rebuilt from previous revision + only CFDI blocks). Lesson: verify each commit in a clean `git worktree`, not the working tree. Core HEAD and API HEAD both verified clean that way. API resolves Core through a local `replace` to the (dirty) Core working tree, so API is not yet verified against Core HEAD alone.
- [x] T7 (commits e589857 script, + integration test) `scripts/db/disposable_test.sh` runs go test on a throwaway `garfex_c3b_disposable_*` DB inside the compose Postgres; `supplier_by_tax_id_integration_test.go` covers the RFC lookup on real Postgres. Compiles and skips without DSNs. EXECUTED by the user against real Postgres (disposable DB, 10 migrations): 7/7 cases + SQL contract PASS after fixing one test (btrim trims spaces only, not tabs; tab/newline trimming is the service's job, covered in app tests). Fix commit 09b4b7e. Core commits: f6c1805, 56213db, 2f18664, e589857, 4c603a5, 09b4b7e.
- Materials/prices (later, not this phase): same shape but preview+apply, idempotent by stamp UUID; rules undefined yet.
- Engram MCP writes fail (8 active runtime sessions in this dir, mem_doctor 2026-09-18); mirror via `engram save` CLI works.

## Acceptance criteria
- All 4 sample CFDIs parse; concept counts 12/14/18/4; Emisor RFC/Nombre/Regimen, UUID, totals match the XML.
- Non-CFDI / wrong version / missing Emisor RFC -> typed error with stable code, no panic.
- Endpoint: 200 with data; 400 on missing/unreadable file; 422 on non-CFDI; body over limit -> 400.
- OpenAPI describes the path; router test path list updated.

## Progress / evidence
Core branch feat/cfdi-xml-parser: f6c1805, 56213db, 2f18664. API branch feat/cfdi-parse-endpoint: 3da6a46 (broken openapi), 93bb860, 9ea1e35 (repair). RDD off, no native review. Nothing pushed, no PR.
Before deploy: tag Core (e.g. v0.3.0), bump garfex-api require and drop the local `replace` (already flagged TEMPORARY in go.mod). garfex-api dev server needs a manual restart to expose the route.

## Delivery (Core, 2026-09-18)
Strategy: independent PRs to main (slices do not depend on each other; same pattern as #173-#177). Local main holds 3 unpushed user commits (6cffb9e, a28d932, 7f0c297) so slices were rebuilt on origin/main and verified in a clean worktree.
- PR #178 feat/cfdicore-parser (e0fa466, 7d9761c): 781 lines, size:exception requested (one cohesive unit).
- PR #179 feat/suppliercore-tax-id-lookup (2135403, 841c02f): 382 lines.
- PR #180 test/disposable-db-script (350a945): 77 lines.
No issues created (previous PRs used `Closes #N`); no AI footer in PR bodies (user rule).
API (2 commits on feat/cfdi-parse-endpoint, base a4df3b5 is an UNPUSHED user commit): NOT pushed. Blocked until Core #178/#179 are merged and Core is tagged (API needs cfdicore + GetSupplierByTaxIdentifier), then drop the go.mod replace. Old local branch feat/cfdi-xml-parser and backup/cfdi-parse-endpoint-pre-rewrite (API) are disposable after merge.

## Merged (2026-09-19)
Core PRs #178, #179, #180 rebase-merged into origin/main (2d93a7b, 6b52482, 12b1733, 117f4ea, 308299c); CI green; clean-worktree `go test -race ./...` on origin/main: 15 packages ok.
API BLOCKED, verified: API HEAD does not compile against Core origin/main (`catalog_descriptors.go:145 field.AllowCreate undefined`): API base commit a4df3b5 needs Core's UNPUSHED 7f0c297 (user's other feature). To land API: land user's 3 Core commits (6cffb9e, a28d932, 7f0c297) + 2 API commits (492aed2, a4df3b5), tag Core, bump API require, drop replace.

## LANDED IN MAIN (2026-09-19)
Core (origin/main 5694a08, tag v0.3.0 annotated, CI green): PRs #178 cfdicore, #179 supplier tax-id lookup, #180 disposable-db script, #181 AllowCreate (user's 7f0c297+a28d932), #182 PresentationField.Active (user's 6cffb9e). All rebase-merged.
API (origin/main 2be4cc9, no `replace`, require v0.3.0, clean-worktree tests pass): PRs #1 (bump + user's 492aed2/a4df3b5 + gofmt), #2 /v1/cfdi/parse, #3 /v1/suppliers/from-cfdi/preview. API repo has no CI; verified in clean worktrees against the real v0.3.0 module.
Local `main` of both repos fast-forwarded to origin/main (patch-equivalence checked; a4df3b5 differed only by go.mod/go.sum + message). Working tree still on old feature branches with the user's uncommitted attribute-order work: untouched.
Disposable now: local branches feat/cfdi-xml-parser (Core), feat/cfdi-parse-endpoint + backup/cfdi-parse-endpoint-pre-rewrite (API); remote branches of merged PRs (deleteBranchOnMerge=false).
Lesson: verify each commit in a clean worktree; shared files mixed with another feature's uncommitted work broke a commit once (3da6a46).

## Cleanup (2026-09-19)
Deleted (only branches created in this session; user's older remote branches untouched): Core local feat/cfdi-xml-parser + 5 slice branches, remote feat/cfdicore-parser, feat/suppliercore-tax-id-lookup, test/disposable-db-script, feat/resourcecore-allow-create, fix/presentation-field-active; API local backup/cfdi-parse-endpoint-pre-rewrite, feat/cfdi-parse-endpoint + 3 slice branches, remote feat/catalog-error-code-and-reuse-metadata, feat/cfdi-parse-endpoint-v2, feat/supplier-cfdi-preview. Both checkouts moved to main; uncommitted files proven byte-identical by fingerprint (Core 34 paths, API 8 paths). Old a4df3b5 with the go.mod `replace` survives only in the reflog.

## Next step
DONE backend. Remaining: frontend button (sistema-ui), materials/prices rules. Frontend flow: upload -> POST /v1/suppliers/from-cfdi/preview -> if existing: open it; else user completes tradeName and POST /v1/suppliers. Open: push/PR strategy (2 repos; API verified against the dirty Core working tree via local replace, not Core HEAD alone; Core needs a tag before the API can drop the replace), materials/prices rules.
