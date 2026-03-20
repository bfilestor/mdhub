#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ -f "$ROOT_DIR/app/go-api/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT_DIR/app/go-api/.env"
  set +a
fi
if [[ -f "$ROOT_DIR/app/nextjs-admin/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT_DIR/app/nextjs-admin/.env"
  set +a
fi

export MDHUB_API_TOKEN="${MDHUB_API_TOKEN:-dev-token}"
export MDHUB_API_PORT="${MDHUB_API_PORT:-8080}"
export MDHUB_API_BASE_URL="${MDHUB_API_BASE_URL:-http://127.0.0.1:${MDHUB_API_PORT}}"
export PORT="${PORT:-3100}"

echo "[mdhub] API      : http://127.0.0.1:${MDHUB_API_PORT}"
echo "[mdhub] Admin    : http://127.0.0.1:${PORT}"
echo "[mdhub] API base : ${MDHUB_API_BASE_URL}"

(
  cd "$ROOT_DIR/app/go-api/cmd/api"
  go run .
) &
API_PID=$!

cleanup() {
  echo "[mdhub] stopping..."
  kill "$API_PID" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

cd "$ROOT_DIR/app/nextjs-admin"
npm run dev -- --port "$PORT"
