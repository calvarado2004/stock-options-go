#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

BACKEND_PORT="${BACKEND_PORT:-18080}"
cleanup() {
  BACKEND_PORT="$BACKEND_PORT" docker compose down >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "[integration] building and starting backend in docker compose"
BACKEND_PORT="$BACKEND_PORT" docker compose up -d --build backend

echo "[integration] waiting for backend health"
for i in {1..30}; do
  if curl -fsS "http://localhost:${BACKEND_PORT}/data?ticker=PSTG" >/dev/null 2>&1; then
    break
  fi
  sleep 2
  if [[ "$i" == "30" ]]; then
    echo "backend did not become ready in time"
    BACKEND_PORT="$BACKEND_PORT" docker compose logs backend
    exit 1
  fi
done

echo "[integration] running ingest/data/forecast checks"
INGEST_RESPONSE="$(curl -fsS -X POST "http://localhost:${BACKEND_PORT}/ingest?ticker=AAPL")"
echo "$INGEST_RESPONSE"

echo "$INGEST_RESPONSE" | grep -q '"ticker":"AAPL"'
echo "$INGEST_RESPONSE" | grep -q '"provider_used"'

DATA_RESPONSE="$(curl -fsS "http://localhost:${BACKEND_PORT}/data?ticker=AAPL")"
echo "$DATA_RESPONSE" | grep -q '"data_count":'

FORECAST_RESPONSE="$(curl -fsS "http://localhost:${BACKEND_PORT}/forecast?ticker=AAPL")"
echo "$FORECAST_RESPONSE" | grep -q '"ticker":"AAPL"'

echo "[integration] docker integration checks passed"
