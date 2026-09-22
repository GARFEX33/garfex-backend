# Rename and Run Local Backend

## Objective

Rename the consolidated repository, local directory, Go module, and imports to `garfex-backend`, then provide one command that starts PostgreSQL, applies migrations, and runs the API on the existing frontend proxy target.

## Scope

- Rename GitHub repository `GARFEX33/garfex-costos-unitarios` to `GARFEX33/garfex-backend`.
- Rename the local directory to `garfex-backend`.
- Rename the Go module and all repository-owned imports to `github.com/GARFEX33/garfex-backend`.
- Add a backend-owned command that starts the database, applies migrations, and runs `cmd/api`.
- Add the required DSN/listen variables to the safe environment template and document setup.
- Run the API on `127.0.0.1:8090`, matching the existing external frontend proxy.
- Verify direct backend health and representative `/v1` reads, then perform an external proxy request without inspecting or modifying frontend code.

## Out of Scope

- Inspecting, editing, testing, cleaning, switching, or generating files in any frontend repository.
- Starting Convex or changing frontend proxy configuration.
- Stopping unrelated containers when free ports are available.
- Adding Temporal or Jev.
- Destructive database resets.

## Constraints

- Never disclose `.env` values or commit secrets.
- Do not run local `go build` or race tests; builds remain CI-only.
- PostgreSQL remains the source of truth.
- Keep process logs and PID files outside repositories.
- Preserve public Core behavior while intentionally changing its module import path.
- Use one feature branch and reviewable Conventional Commits.

## Tasks

### T1 — Map backend runtime and rename scope

- Status: done.
- Route: delegated read-only exploration plus parent runtime-state inspection.
- Evidence:
  - User selected `garfex-backend` for GitHub repository and local directory.
  - User selected a full module/import rename to `github.com/GARFEX33/garfex-backend`.
  - UI is explicitly outside scope; its owner confirmed no backend-side UI changes are needed.
  - PostgreSQL is healthy on `127.0.0.1:5432`; migration version is 14 with `dirty=false`.
  - API successfully started on `127.0.0.1:8090` and returned successful catalog, resource, and supplier reads.

### T2 — Rename backend and add one-command startup

- Status: done.
- Route: delegated writer; multi-file module rename and script/documentation changes.
- Allowed edit surfaces:
  - `go.mod`
  - repository-owned `*.go` files containing the old module path
  - `.env.example`
  - `README.md`
  - `scripts/dev-backend.sh`
  - tests or fixtures that assert the module/repository name
  - this task document
- Outcome:
  - No active repository-owned Go import, module metadata, or README example references `github.com/GARFEX33/garfex-costos-unitarios`; historical archives and prior task records remain unchanged.
  - `sh scripts/dev-backend.sh` loads safe local configuration, starts the database, applies migrations with `garfex_admin`, and runs the API with `garfex_app`.
  - The command defaults the API to `127.0.0.1:8090` without changing the executable's general default.
- Checks:
  - Script uses explicit DSNs and does not print secrets.
  - Script validates prerequisites and preserves the database volume.
  - `sh -n` and ShellCheck pass.
  - README and `.env.example` match runtime behavior.
- Implementation evidence: module/import examples and metadata renamed; executable local launcher, isolated fake-command Go tests, configurable persistent volume, safe template block, and README updated.
- Work-unit commit: `5287c0c` (`feat!: rename backend and add local startup`).
- Observed checks: independent verifier passed `gofmt -l .`, `go vet ./...`, `golangci-lint run ./...`, `GOWORK=off go test ./... -count=1` (1,837 tests in 25 packages), `go test ./scripts -count=1` (7 tests), `sh -n scripts/dev-backend.sh`, safe `docker compose config -q`, and `git diff --check`. ShellCheck was unavailable and is explicitly skipped. The verifier confirmed all active Go changes are import-prefix substitutions only and approved the corrected version documentation after confirming existing tags through `v0.4.0` retain the prior module identity.

### T3 — Launch and verify backend integration

- Status: done.
- Route: runtime operations and delegated verification.
- Checks:
  - One command starts or reuses healthy PostgreSQL, applies migrations idempotently, and runs the API.
  - `GET /healthz` succeeds directly on port 8090.
  - Representative catalog, resource, and supplier `/v1` reads succeed.
  - An external request through the already-running frontend proxy succeeds; no frontend files are inspected or modified.
- Runtime evidence:
  - Work-unit commit `5287c0c` contains the verified launcher and runtime configuration.
  - `sh scripts/dev-backend.sh` was launched as the foreground command under a host supervisor; its PID is recorded in `/tmp/garfex-backend.pid` and output in `/tmp/garfex-backend.log`.
  - Compose project `garfex-backend` created healthy container `garfex-backend-db-1` while reusing volume `garfex-costos-unitarios-workspace_garfex_pgdata`.
  - Migration state remained `14|false` after the idempotent migration run.
  - Direct `GET` requests to health, catalog descriptors/classes, resources, and suppliers on `127.0.0.1:8090` all returned HTTP 200.
  - The existing external frontend proxy at `127.0.0.1:5174` returned HTTP 200 for `GET /v1/resources?limit=1`; its owning session confirmed no UI files were inspected or modified and no service was started.
  - The backend log contains no `postgres://` pattern.

### T4 — Deliver renamed backend

- Status: done.
- Route: independent verification, work-unit commits, issue-first pull request, CI, GitHub rename, release, and local-directory rename.
- Delivery evidence:
  - Local format, vet, lint, full tests, script tests, shell syntax, Compose config, and diff checks passed; ShellCheck remained unavailable.
  - Issue #195 was explicitly approved and closed by merged PR #196.
  - The user explicitly approved the single-PR size exception; PR #196 carried `size:exception` and `type:breaking-change`.
  - Work-unit commits `5287c0c` and `f7e0bd9` merged to `main` as `1096434`.
  - PR and push CI passed; default-branch run `35792646060` passed formatting, vet, lint, race tests, explicit API build, and full build.
  - GitHub repository and `origin` are `GARFEX33/garfex-backend`.
  - Release `v0.5.0` is the first tag with module path `github.com/GARFEX33/garfex-backend`.
  - The canonical local directory is `/home/garfex/PROGRAMACION/garfex-backend`.
  - Start: `sh scripts/dev-backend.sh`. Stop the foreground API with Ctrl-C; stop PostgreSQL without deleting data with the same Compose project and `docker compose stop db`.
  - Runtime URL: `http://127.0.0.1:8090`; PID and log files are `/tmp/garfex-backend.pid` and `/tmp/garfex-backend.log` for the supervised local run.

## Progress

- 2026-09-22: User authorized completing and starting the backend while explicitly excluding all UI inspection or modification.
- 2026-09-22: User selected `garfex-backend` for the repository and local directory and approved renaming the Go module/imports.
- 2026-09-22: Active external UI owner confirmed its proxy targets `http://localhost:8090`; no UI change is required.
- 2026-09-22: PostgreSQL is healthy and schema migration 14 is clean.
- 2026-09-22: API runtime smoke test passed on port 8090 with successful health, catalog, resource, and supplier responses.
- 2026-09-22: The new one-command launcher reused the preserved database volume; direct backend reads and an external read through the existing frontend proxy returned HTTP 200.
- 2026-09-22: Issue #195 was created and explicitly approved. The user authorized one PR with a size exception, full GitHub delivery and repository rename, and a post-merge `v0.5.0` release.
- 2026-09-22: PR #196 and default-branch CI passed, the change merged as `1096434`, the repository and local directory were renamed to `garfex-backend`, and release `v0.5.0` was published.

## Next Step

Feature complete. Keep the backend running at `127.0.0.1:8090` for local frontend integration.
