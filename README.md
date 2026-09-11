# Garfex API

Local HTTP entry point for GARFEX Costos Unitarios.

## Safety boundary

This service binds only to `127.0.0.1` or `::1` with a nonzero numeric port.
It has no authentication and is not production-ready. Do not expose it publicly.
Its scope is startup, operational endpoints, and catalog descriptors. It does not
publish catalog records, resources, or supplier APIs.

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

- `GET /healthz` is liveness only; it is not a database/readiness probe.
- `GET /v1/catalog/descriptors` returns dynamic Core catalog metadata only; it does not list catalog records.
- `GET /openapi.yaml` serves the embedded OpenAPI document.
- `GET /docs` serves Scalar API documentation.

The Scalar page loads a pinned external browser dependency from jsDelivr
(`@scalar/api-reference@1.25.0`); interactive docs require browser network access.
