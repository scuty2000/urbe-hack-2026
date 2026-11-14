#!/usr/bin/env bash
# Reset the demo dataset.
#
# Preferred path: log in as the demo admin through the running API and call
# POST /api/demo/reset with the X-Demo-Reset-Secret header. The secret is read
# from .env (or the environment) and is never printed.
#
# Fallback path: when API_DEMO_RESET_SECRET is missing or the API is not
# reachable, seed the local SQLite file directly with `go run ./cmd/seed`.
#
# Usage:
#   scripts/demo-reset.sh            # HTTP reset, seed fallback
#   scripts/demo-reset.sh --seed     # force the local seed
#   DEMO_API_URL=http://host:3000 scripts/demo-reset.sh
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

# Load .env without sourcing it: values such as API_CORS_ALLOWEDORIGINS contain
# semicolons, which a shell `source` would interpret as command separators.
load_env() {
  [[ -f .env ]] || return 0

  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ -z "$line" || "$line" == \#* ]] && continue
    [[ "$line" != *=* ]] && continue

    key=${line%%=*}
    value=${line#*=}

    if [[ ${#value} -ge 2 ]]; then
      first=${value:0:1}
      last=${value: -1}
      if [[ ( "$first" == '"' && "$last" == '"' ) || ( "$first" == "'" && "$last" == "'" ) ]]; then
        value=${value:1:${#value}-2}
      fi
    fi

    export "$key=$value"
  done < .env
}

load_env

API_BASE_URL="${DEMO_API_URL:-http://localhost:3000}"

seed_locally() {
  echo "Seeding the local SQLite database with 'go run ./cmd/seed'..."
  exec go run ./cmd/seed
}

if [[ "${1:-}" == "--seed" || "${1:-}" == "seed" ]]; then
  seed_locally
fi

if [[ -z "${API_DEMO_RESET_SECRET:-}" ]]; then
  echo "API_DEMO_RESET_SECRET is not set; falling back to the local seed." >&2
  seed_locally
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required for the HTTP reset; falling back to the local seed." >&2
  seed_locally
fi

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
umask 077

# The secret travels in a 0600 header file instead of the process argument list.
printf 'X-Demo-Reset-Secret: %s\n' "$API_DEMO_RESET_SECRET" >"$workdir/reset-header"

login_status="$(curl -sS \
  -o "$workdir/login.json" \
  -w '%{http_code}' \
  -c "$workdir/cookies" \
  -H 'Content-Type: application/json' \
  -d '{"userId":"admin-demo"}' \
  "$API_BASE_URL/api/auth/login" 2>"$workdir/curl.err")" || {
  echo "The API at $API_BASE_URL is not reachable; falling back to the local seed." >&2
  seed_locally
}

if [[ "$login_status" != "200" ]]; then
  echo "Admin login failed with HTTP $login_status at $API_BASE_URL:" >&2
  cat "$workdir/login.json" >&2
  echo >&2
  echo "Start the API first (scripts/dev-up.sh or docker compose up -d api), or use --seed." >&2
  exit 1
fi

reset_status="$(curl -sS \
  -o "$workdir/reset.json" \
  -w '%{http_code}' \
  -b "$workdir/cookies" \
  -H "@$workdir/reset-header" \
  -X POST \
  "$API_BASE_URL/api/demo/reset")"

if [[ "$reset_status" == "200" ]]; then
  echo "Demo dataset reset via $API_BASE_URL."
  exit 0
fi

if [[ "$reset_status" == "503" ]]; then
  echo "The API has no demo reset configured; falling back to the local seed." >&2
  seed_locally
fi

echo "Demo reset failed with HTTP $reset_status:" >&2
cat "$workdir/reset.json" >&2
echo >&2
if [[ "$reset_status" == "403" ]]; then
  echo "Check that API_DEMO_RESET_SECRET in .env matches the secret used by the running API." >&2
fi
exit 1
