#!/usr/bin/env bash
# Start the Cloudflare Tunnel that publishes the local API.
#
# A named tunnel is required: CLOUDFLARE_TUNNEL_TOKEN must be set in .env (or
# the environment) and the public hostname must be routed to http://api:3000 in
# the Cloudflare Zero Trust dashboard. The token is never printed or committed.
#
# Usage:
#   scripts/tunnel.sh
#   DEMO_PUBLIC_URL=https://demo.example.org scripts/tunnel.sh
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

if [[ -z "${CLOUDFLARE_TUNNEL_TOKEN:-}" ]]; then
  cat >&2 <<'EOF'
CLOUDFLARE_TUNNEL_TOKEN is not set.

A public demo URL needs a Cloudflare named tunnel:
  1. Create a tunnel in Cloudflare Zero Trust -> Networks -> Tunnels.
  2. Add a public hostname that routes to http://api:3000.
  3. Copy the tunnel token into .env as CLOUDFLARE_TUNNEL_TOKEN=...
  4. Re-run scripts/tunnel.sh.

The token is a secret: do not commit it and do not paste it in the video.
EOF
  exit 1
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required to run the tunnel container." >&2
  exit 1
fi

if ! docker info >/dev/null 2>&1; then
  echo "The Docker daemon is not reachable. Start Docker and retry." >&2
  exit 1
fi

docker compose --profile tunnel up -d tunnel

public_url="${DEMO_PUBLIC_URL:-}"
if [[ -n "$public_url" ]]; then
  public_url="${public_url%/}"
  echo "Tunnel started. Public demo URL: $public_url"
  if command -v curl >/dev/null 2>&1; then
    health_status="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 10 "$public_url/healthz" 2>/dev/null)" || health_status="unreachable"
    echo "Health check: $public_url/healthz -> $health_status"
    if [[ "$health_status" != "200" ]]; then
      echo "warning: the public health check did not return 200; verify the Cloudflare hostname routes to http://api:3000." >&2
    fi
  fi
else
  echo "Tunnel started. The public URL is the hostname configured in the Cloudflare" \
    "dashboard for this tunnel (route it to http://api:3000)."
  echo "Set DEMO_PUBLIC_URL=https://... in .env to have this script print and check it."
fi
echo "Tunnel logs: docker compose logs -f tunnel"
echo "Stop it with: docker compose --profile tunnel stop tunnel"
