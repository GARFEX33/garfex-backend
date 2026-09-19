# Garfex API

Local HTTP entry point for GARFEX Costos Unitarios.

## Safety boundary

This service binds only to `127.0.0.1` or `::1` with a nonzero numeric port.
It has no authentication, no CORS headers, and is not production-ready. Do not
expose it publicly.

## Run locally

Set an explicit `GARFEX_API_DSN`; blank or omitted values are rejected. The DSN
is passed unchanged to Core. pgx applies documented `PG*` environment defaults to
connection parameters omitted from that DSN. Core connects eagerly to PostgreSQL
and expects its runtime `garfex_app` database role.

`GARFEX_API_LISTEN_ADDR` is optional and defaults to `127.0.0.1:8080`. Only
`127.0.0.1:PORT` and `[::1]:PORT` are accepted. For example:

```sh
GARFEX_API_DSN='postgres://garfex_app@localhost/garfex' go run ./cmd/garfex-api
```

The module uses Go 1.26.5 and a local replace to the sibling
`../garfex-costos-unitarios-workspace` Core checkout. Keep that sibling available
when building or running this API.

## Current endpoints

The full, authoritative list — with request/response schemas — is the embedded
OpenAPI document (`GET /openapi.yaml`), browsable interactively at `GET /docs`
(Scalar). This section is a map, not the contract.

- `GET /healthz` is liveness only; it is not a database/readiness probe.
- **Catalog** — generic across all 11 registered kinds (`CLASE`, `FAMILIA`,
  `TIPO`, `CARACTERISTICA`, `CONJUNTO_OPCIONES`, `OPCION`,
  `RELACION_OPCIONES`, `UNIDAD`, `POLITICA_UNIDAD`, `APLICABILIDAD`,
  `PRESENTACION`): descriptors, active classes, list/get, create, update,
  deactivate/reactivate, and permanent (hard) delete.
- **Resource** — search/get/describe by natural key (class + identity-v1),
  create, update, and deactivate/reactivate by internal numeric id.
- **Supplier** — search/get/update by id, create, and its nested branches and
  contacts (read-only).
- **CFDI** — `POST /v1/cfdi/parse` reads a CFDI 4.0 XML (multipart `file` field
  or raw body) and returns all its data plus a `supplierDraft` built from the
  Emisor, to prefill a supplier form. Stateless: nothing is stored.

The Scalar page loads a pinned external browser dependency from jsDelivr
(`@scalar/api-reference@1.25.0`); interactive docs require browser network access.

## Local testing

`./run-dev.sh` (gitignored; reads the sibling Core workspace's `.env` for the
`garfex_app` DB credentials) builds the DSN and starts the server on
`127.0.0.1:8090`. Then open `http://127.0.0.1:8090/docs` in a browser and use
Scalar's "Test Request" panel to call any endpoint without a separate HTTP
client.
