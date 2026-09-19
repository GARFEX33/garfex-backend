#!/usr/bin/env sh
# Runs the Go tests against a throwaway database created inside the local
# compose PostgreSQL (service `db`), then drops it. The database in POSTGRES_DB
# is never touched.
#
# usage: sh scripts/db/disposable_test.sh [go test arguments]
#   sh scripts/db/disposable_test.sh                      # go test ./...
#   sh scripts/db/disposable_test.sh ./internal/postgres/ -run SupplierByTaxID -v
#
# Requires a running `db` service (docker compose up -d --wait db) and a .env
# with the bootstrap and role passwords. Passwords are placed in a URL without
# escaping, so avoid characters such as @ : / ? # in them.
set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repository_root=$(CDPATH='' cd -- "$script_dir/../.." && pwd)
cd "$repository_root"

if [ ! -f .env ]; then
  printf '%s\n' ".env not found; copy .env.example and fill it in" >&2
  exit 64
fi
set -a
# shellcheck disable=SC1091
. ./.env
set +a
: "${POSTGRES_USER:?POSTGRES_USER must be set}"
: "${POSTGRES_DB:?POSTGRES_DB must be set}"
: "${GARFEX_ADMIN_PASSWORD:?GARFEX_ADMIN_PASSWORD must be set}"
: "${GARFEX_APP_PASSWORD:?GARFEX_APP_PASSWORD must be set}"

# Some integration tests refuse any database without this prefix.
db_name="garfex_c3b_disposable_$(date +%s)_$$"
case $db_name in
  "$POSTGRES_DB") printf '%s\n' "refusing to use the main database" >&2; exit 1 ;;
esac

container=$(docker compose ps -q db)
if [ -z "$container" ]; then
  printf '%s\n' "db service is not running; run: docker compose up -d --wait db" >&2
  exit 1
fi

psql_bootstrap() {
  docker compose exec -T db psql --set=ON_ERROR_STOP=1 \
    --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" "$@"
}

cleanup() {
  if psql_bootstrap --command "DROP DATABASE IF EXISTS \"$db_name\" WITH (FORCE)" >/dev/null 2>&1; then
    printf '%s\n' "dropped $db_name"
  else
    printf '%s\n' "WARNING: could not drop $db_name; drop it manually" >&2
  fi
}
trap cleanup EXIT HUP INT TERM

printf '%s\n' "creating $db_name"
psql_bootstrap --command "CREATE DATABASE \"$db_name\"" >/dev/null

# Same role and grant setup the main database got at bootstrap.
docker compose exec -T -e POSTGRES_DB="$db_name" db sh /docker-entrypoint-initdb.d/00-roles.sh >/dev/null

# migrate.sh reaches the database through the compose network, by host `db`.
network=$(docker inspect --format '{{range $name, $_ := .NetworkSettings.Networks}}{{$name}}{{end}}' "$container")
INTEGRATION_PROJECT_NAME=${network%_default} \
  GARFEX_ADMIN_DSN="postgres://garfex_admin:${GARFEX_ADMIN_PASSWORD}@db:5432/${db_name}?sslmode=disable" \
  sh "$script_dir/migrate.sh" up

GARFEX_TEST_DSN="postgres://garfex_app:${GARFEX_APP_PASSWORD}@127.0.0.1:5432/${db_name}?sslmode=disable"
GARFEX_ADMIN_TEST_DSN="postgres://garfex_admin:${GARFEX_ADMIN_PASSWORD}@127.0.0.1:5432/${db_name}?sslmode=disable"
export GARFEX_TEST_DSN GARFEX_ADMIN_TEST_DSN

if [ "$#" -eq 0 ]; then
  set -- ./...
fi
go test -count=1 "$@"
