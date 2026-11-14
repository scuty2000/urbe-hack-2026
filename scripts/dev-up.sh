#!/usr/bin/env bash
set -euo pipefail

if [[ ! -f .env ]]; then
  echo "error: .env not found (copy .env.example to .env and fill it in)" >&2
  exit 1
fi

# Load .env without sourcing it: values such as API_CORS_ALLOWEDORIGINS contain
# semicolons, which a shell `source` would interpret as command separators.
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

docker compose up -d qdrant
exec go run ./cmd/webapi/ -c demo/config.yml
