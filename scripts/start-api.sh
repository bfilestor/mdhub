#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
API_DIR="$ROOT_DIR/app/go-api/cmd/api"

if [[ -f "$ROOT_DIR/app/go-api/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT_DIR/app/go-api/.env"
  set +a
fi

export MDHUB_API_TOKEN="${MDHUB_API_TOKEN:-dev-token}"
export MDHUB_API_PORT="${MDHUB_API_PORT:-8080}"

cd "$API_DIR"
echo "[mdhub] starting go-api on :$MDHUB_API_PORT"
go run .
