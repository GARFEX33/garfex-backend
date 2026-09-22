#!/bin/sh
set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
root=$(CDPATH='' cd -- "$script_dir/.." && pwd)
env_file=${GARFEX_ENV_FILE:-$root/.env}

if [ ! -f "$env_file" ]; then
  printf '%s\n' 'backend environment file is missing' >&2
  exit 64
fi
set -a
# The environment file is a trusted local POSIX shell configuration.
. "$env_file"
set +a

if [ -z "${GARFEX_ADMIN_DSN:-}" ] || [ -z "${GARFEX_API_DSN:-}" ]; then
  printf '%s\n' 'GARFEX_ADMIN_DSN and GARFEX_API_DSN are required' >&2
  exit 64
fi
GARFEX_API_LISTEN_ADDR=${GARFEX_API_LISTEN_ADDR:-127.0.0.1:8090}
COMPOSE_PROJECT_NAME=${COMPOSE_PROJECT_NAME:-garfex-backend}
export COMPOSE_PROJECT_NAME GARFEX_ADMIN_DSN GARFEX_API_DSN GARFEX_API_LISTEN_ADDR

# Keep Compose interpolation tied to the same configuration even from another cwd.
docker compose --project-directory "$root" --env-file "$env_file" -p "$COMPOSE_PROJECT_NAME" up -d --wait db
INTEGRATION_PROJECT_NAME=$COMPOSE_PROJECT_NAME GARFEX_REPOSITORY_ROOT=$root \
  sh "$root/scripts/db/migrate.sh" up
cd "$root"
exec env -i PATH="$PATH" HOME="${HOME:-}" GOWORK=off \
  GARFEX_API_DSN="$GARFEX_API_DSN" GARFEX_API_LISTEN_ADDR="$GARFEX_API_LISTEN_ADDR" \
  go run ./cmd/api
