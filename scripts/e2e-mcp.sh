#!/usr/bin/env bash
# End-to-end MCP test: starts docker-compose, runs the Go MCP client test suite.
# Usage: bash scripts/e2e-mcp.sh
# Prerequisites: docker, go, curl, jq

set -euo pipefail

BASE_URL="${AGENTMEMORY_E2E_BASE_URL:-http://localhost:8080}"
TEST_EMAIL="e2e-mcp-$(date +%s)@test.local"
TEST_PASSWORD="testpass123"
TEST_NAME="E2E MCP Test User"

cleanup() {
  echo ""
  echo "==> Cleaning up..."
  docker compose down --volumes 2>/dev/null || true
}
trap cleanup EXIT

# ── start stack ─────────────────────────────────────────────────────────────

echo "==> Starting docker compose..."
docker compose down --volumes 2>/dev/null || true

# Override scheduler intervals to fire within ~1 minute for e2e testing
export COMPRESSION_INTERVAL_MINUTES=1m
export SUMMARIZATION_INTERVAL_MINUTES=1m
export CONSOLIDATION_INTERVAL_MINUTES=1m
export REFLECTION_INTERVAL_MINUTES=1m

docker compose up -d --build

echo "==> Waiting for health..."
for i in $(seq 1 30); do
  if curl -sf "$BASE_URL/health" > /dev/null 2>&1; then break; fi
  sleep 1
done

# ── create user and API key ─────────────────────────────────────────────────

echo "==> Creating test user..."
COOKIE_JAR=$(mktemp /tmp/e2e-mcp-cookies.XXXXXX)

# Get CSRF cookie
curl -sf -c "$COOKIE_JAR" "$BASE_URL/health" > /dev/null
CSRF=$(grep csrf_token "$COOKIE_JAR" | awk '{print $NF}')

# Register
curl -sf -b "$COOKIE_JAR" -c "$COOKIE_JAR" \
  -H "X-CSRF-Token: $CSRF" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$TEST_EMAIL\",\"password\":\"$TEST_PASSWORD\",\"name\":\"$TEST_NAME\"}" \
  "$BASE_URL/v1/auth/register" > /dev/null

# Login
RESP=$(curl -sf -b "$COOKIE_JAR" -c "$COOKIE_JAR" \
  -H "X-CSRF-Token: $CSRF" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$TEST_EMAIL\",\"password\":\"$TEST_PASSWORD\"}" \
  "$BASE_URL/v1/auth/login")
SESSION_TOKEN=$(echo "$RESP" | jq -r '.token')

# Refresh CSRF (GET /v1/auth/me sets a new cookie)
curl -sf -b "$COOKIE_JAR" -c "$COOKIE_JAR" \
  -H "Authorization: Bearer $SESSION_TOKEN" \
  "$BASE_URL/v1/auth/me" > /dev/null
CSRF=$(grep csrf_token "$COOKIE_JAR" | tail -1 | awk '{print $NF}')

# Create API key
RESP=$(curl -sf -b "$COOKIE_JAR" -c "$COOKIE_JAR" \
  -H "X-CSRF-Token: $CSRF" \
  -H "Authorization: Bearer $SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"label":"e2e-mcp-test"}' \
  "$BASE_URL/v1/auth/keys")
API_KEY=$(echo "$RESP" | jq -r '.key')
rm -f "$COOKIE_JAR"

echo "==> API key obtained: ${API_KEY:0:20}..."

# ── run Go MCP e2e tests ────────────────────────────────────────────────────

echo ""
echo "==> Running MCP e2e tests..."
export AGENTMEMORY_E2E_API_KEY="$API_KEY"
export AGENTMEMORY_E2E_BASE_URL="$BASE_URL"

go test -tags=e2e -v -timeout 10m ./tests/e2e/

echo ""
echo "All MCP e2e tests passed."
