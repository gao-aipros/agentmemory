#!/usr/bin/env bash
# End-to-end test: starts docker-compose, exercises the full REST API + DB verification.
# Usage: bash scripts/e2e-docker.sh
# Prerequisites: docker, curl, jq

set -euo pipefail

BASE_URL="http://localhost:8080"
COOKIE_JAR=$(mktemp /tmp/e2e-cookies.XXXXXX)
PASSED=0
FAILED=0
TEST_EMAIL="e2e-$(date +%s)@test.local"
TEST_PASSWORD="testpass123"
TEST_NAME="E2E Test User"

cleanup() {
  echo ""
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "Results: $PASSED passed, $FAILED failed"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  rm -f "$COOKIE_JAR"
  docker compose down --volumes 2>/dev/null || true
}
trap cleanup EXIT

# ── helpers ────────────────────────────────────────────────────────────────

pass() { echo "  ✅ $1"; PASSED=$((PASSED + 1)); }
fail() { echo "  ❌ $1 — $2"; FAILED=$((FAILED + 1)); exit 1; }

assert_status() {
  local got=$1 expected=$2 step=$3
  if [ "$got" -eq "$expected" ]; then pass "$step (HTTP $got)"; else fail "$step" "expected HTTP $expected, got $got"; fi
}

assert_json() {
  local body=$1 jq_filter=$2 expected=$3 step=$4
  local got
  got=$(echo "$body" | jq -r "$jq_filter" 2>/dev/null || echo "<parse error>")
  if [ "$got" = "$expected" ]; then pass "$step"; else fail "$step" "expected '$expected', got '$got'"; fi
}

db_query() {
  docker compose exec -T db psql -U agentmemory -d agentmemory -t -A -c "$1" 2>/dev/null | tr -d '[:space:]'
}

# ── step 0: start stack ────────────────────────────────────────────────────

echo "==> Starting docker compose..."
docker compose down --volumes 2>/dev/null || true
docker compose up -d --build

echo "==> Waiting for health endpoint..."
for i in $(seq 1 30); do
  if curl -sf "$BASE_URL/health" > /dev/null 2>&1; then break; fi
  sleep 1
done

# ── step 1: health check ───────────────────────────────────────────────────

echo "--- Step 1: Health check"
RESP=$(curl -sf -c "$COOKIE_JAR" "$BASE_URL/health")
assert_status 200 200 "Health check"
assert_json "$RESP" ".status" "ok" "status is ok"
CSRF=$(grep csrf_token "$COOKIE_JAR" | awk '{print $NF}')

# ── step 2: create admin user ──────────────────────────────────────────────

echo "--- Step 2: Create admin user"
docker compose exec -T agentmemory agentmemory user create \
  --db-url "postgres://agentmemory:agentmemory@db:5432/agentmemory?sslmode=disable" \
  --email "$TEST_EMAIL" \
  --password "$TEST_PASSWORD" \
  --name "$TEST_NAME"

# Verify in DB
USER_COUNT=$(db_query "SELECT COUNT(*) FROM users WHERE email='$TEST_EMAIL';")
if [ "$USER_COUNT" -ge 1 ]; then pass "user row exists in DB"; else fail "user row in DB" "not found"; fi

# ── step 3: login ──────────────────────────────────────────────────────────

echo "--- Step 3: Login"
RESP=$(curl -sf -b "$COOKIE_JAR" -c "$COOKIE_JAR" \
  -H "X-CSRF-Token: $CSRF" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$TEST_EMAIL\",\"password\":\"$TEST_PASSWORD\"}" \
  "$BASE_URL/v1/auth/login")
SESSION_TOKEN=$(echo "$RESP" | jq -r '.token')
assert_status 200 200 "Login"
assert_json "$RESP" ".user.email" "$TEST_EMAIL" "email matches"
# Note: login returns a JWT but does NOT create a session row — that happens in session/start.

# ── step 4: get me ─────────────────────────────────────────────────────────

echo "--- Step 4: Get me"
RESP=$(curl -sf -b "$COOKIE_JAR" -c "$COOKIE_JAR" \
  -H "Authorization: Bearer $SESSION_TOKEN" \
  "$BASE_URL/v1/auth/me")
assert_status 200 200 "Get me"
assert_json "$RESP" ".user.email" "$TEST_EMAIL" "email matches"
# GET /v1/auth/me sets a NEW csrf_token cookie — extract it now for the next POST
CSRF=$(grep csrf_token "$COOKIE_JAR" | tail -1 | awk '{print $NF}')

# ── step 5: create API key ─────────────────────────────────────────────────

echo "--- Step 5: Create API key"
RESP=$(curl -sf -b "$COOKIE_JAR" -c "$COOKIE_JAR" \
  -H "X-CSRF-Token: $CSRF" \
  -H "Authorization: Bearer $SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"label":"e2e-test-key"}' \
  "$BASE_URL/v1/auth/keys")
API_KEY=$(echo "$RESP" | jq -r '.key')
assert_status 201 201 "Create API key"
assert_json "$RESP" ".label" "e2e-test-key" "label matches"

# Verify API key in DB (stored as SHA-256 hash)
KEY_COUNT=$(db_query "SELECT COUNT(*) FROM api_keys WHERE label='e2e-test-key';")
if [ "$KEY_COUNT" -ge 1 ]; then pass "api_key row exists in DB"; else fail "api_key row in DB" "not found"; fi

# ── step 6: start session ──────────────────────────────────────────────────

echo "--- Step 6: Start session"
RESP=$(curl -sf \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{}' \
  "$BASE_URL/v1/api/session/start")
SESSION_ID=$(echo "$RESP" | jq -r '.session_id')
assert_status 201 201 "Start session"
assert_json "$RESP" ".status" "active" "session status active"

# Verify session in DB
DB_STATUS=$(db_query "SELECT status FROM sessions WHERE id='$SESSION_ID';")
if [ "$DB_STATUS" = "active" ]; then pass "session status=active in DB"; else fail "session status in DB" "expected active, got '$DB_STATUS'"; fi

# ── step 7: record observation ─────────────────────────────────────────────

echo "--- Step 7: Record observation"
RESP=$(curl -sf \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d "{
    \"session_id\": \"$SESSION_ID\",
    \"type\": \"user_prompt_submit\",
    \"title\": \"E2E test observation\",
    \"narrative\": \"Observed during automated e2e test.\",
    \"concepts\": [\"e2e\", \"testing\"],
    \"importance\": 0.9
  }" \
  "$BASE_URL/v1/api/observe")
OBS_ID=$(echo "$RESP" | jq -r '.observation_id')
assert_status 201 201 "Record observation"

# Verify observation in DB
OBS_COUNT=$(db_query "SELECT COUNT(*) FROM observations WHERE id='$OBS_ID' AND type='user_prompt_submit';")
if [ "$OBS_COUNT" -ge 1 ]; then pass "observation row exists in DB"; else fail "observation row in DB" "not found"; fi

# ── step 8: end session ────────────────────────────────────────────────────

echo "--- Step 8: End session"
RESP=$(curl -sf \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d "{\"session_id\":\"$SESSION_ID\"}" \
  "$BASE_URL/v1/api/session/end")
assert_status 200 200 "End session"
assert_json "$RESP" ".status" "ended" "session ended"

# Verify session status changed in DB
DB_STATUS=$(db_query "SELECT status FROM sessions WHERE id='$SESSION_ID';")
if [ "$DB_STATUS" = "ended" ]; then pass "session status=ended in DB"; else fail "session status in DB" "expected ended, got '$DB_STATUS'"; fi

# ── step 9: link commit ────────────────────────────────────────────────────

echo "--- Step 9: Link commit"
RESP=$(curl -sf \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d "{
    \"session_id\": \"$SESSION_ID\",
    \"sha\": \"abc123def4567890abcdef1234567890abcdef12\",
    \"branch\": \"main\",
    \"message\": \"test: e2e commit link\"
  }" \
  "$BASE_URL/v1/api/session/commit")
assert_status 200 200 "Link commit"
assert_json "$RESP" ".status" "linked" "commit linked"

# Verify post_commit observation in DB
COMMIT_OBS=$(db_query "SELECT COUNT(*) FROM observations WHERE session_id='$SESSION_ID' AND type='post_commit';")
if [ "$COMMIT_OBS" -ge 1 ]; then pass "post_commit observation exists in DB"; else fail "post_commit observation in DB" "not found"; fi

# ── done ───────────────────────────────────────────────────────────────────

echo ""
echo "All steps passed."
