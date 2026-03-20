#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ADMIN_DIR="$ROOT_DIR/app/nextjs-admin"

if [[ -f "$ADMIN_DIR/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$ADMIN_DIR/.env"
  set +a
fi

export MDHUB_API_BASE_URL="${MDHUB_API_BASE_URL:-http://127.0.0.1:8080}"
export PORT="${PORT:-3100}"

cd "$ADMIN_DIR"
echo "[mdhub] starting nextjs-admin on :$PORT, api=$MDHUB_API_BASE_URL"
npm run start -- --port "$PORT"
