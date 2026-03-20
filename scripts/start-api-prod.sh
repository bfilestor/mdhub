#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
API_DIR="$ROOT_DIR/app/go-api/cmd/api"
API_BIN="$ROOT_DIR/app/go-api/bin/mdhub-api"

if [[ -f "$ROOT_DIR/app/go-api/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT_DIR/app/go-api/.env"
  set +a
fi

export MDHUB_API_TOKEN="${MDHUB_API_TOKEN:-dev-token}"
export MDHUB_API_PORT="${MDHUB_API_PORT:-8080}"

if [[ ! -x "$API_BIN" ]]; then
  echo "[mdhub] api binary not found or not executable: $API_BIN"
  echo "[mdhub] build first: (cd $API_DIR && go build -o ../../bin/mdhub-api .)"
  exit 1
fi

cd "$API_DIR"
echo "[mdhub] starting go-api(binary) on :$MDHUB_API_PORT"
exec "$API_BIN"
