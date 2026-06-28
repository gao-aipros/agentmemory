#!/usr/bin/env bash
# End-to-end test: unified observe + context injection (US3, T018+T020).
# Tests 4 cases against a running agentmemory server, plus hooks.json validation.
#
# Usage:
#   export AGENTMEMORY_E2E_API_KEY="<api-key>"
#   bash scripts/e2e-observe-inject.sh
#
# Prerequisites: curl, jq
# The server must be running and accessible at AGENTMEMORY_E2E_BASE_URL.

set -euo pipefail

BASE_URL="${AGENTMEMORY_E2E_BASE_URL:-http://localhost:8080}"
PASSED=0
FAILED=0

HOOKS_JSON="$(cd "$(dirname "$0")/.." && pwd)/plugin/hooks/hooks.json"

cleanup() {
  # End the test session if we created one
  if [ -n "${SESSION_ID-}" ]; then
    curl -sf -X POST "$BASE_URL/v1/api/session/end" \
      -H "Authorization: Bearer $API_KEY" \
      -H "Content-Type: application/json" \
      -d "{\"session_id\":\"$SESSION_ID\"}" > /dev/null 2>&1 || true
  fi
  echo ""
  echo "----------------------------------------"
  echo "Results: $PASSED passed, $FAILED failed"
  echo "----------------------------------------"
}
trap cleanup EXIT

# --- helpers ---------------------------------------------------------------

pass() { echo "  PASS: $1"; PASSED=$((PASSED + 1)); }
fail() { echo "  FAIL: $1"; FAILED=$((FAILED + 1)); }

assert_json_has() {
  local body=$1 jq_filter=$2 label=$3
  if echo "$body" | jq -e "$jq_filter" > /dev/null 2>&1; then
    pass "$label"
  else
    fail "$label — expected field at $jq_filter"
  fi
}

assert_json_not_has() {
  local body=$1 jq_filter=$2 label=$3
  if echo "$body" | jq -e "$jq_filter" > /dev/null 2>&1; then
    fail "$label — unexpected field at $jq_filter"
  else
    pass "$label"
  fi
}

assert_json_nonempty() {
  local body=$1 jq_filter=$2 label=$3
  local val
  val=$(echo "$body" | jq -r "$jq_filter" 2>/dev/null || echo "")
  if [ -n "$val" ] && [ "$val" != "null" ]; then
    pass "$label"
  else
    fail "$label — expected non-empty value at $jq_filter"
  fi
}

assert_eq() {
  local got=$1 expected=$2 label=$3
  if [ "$got" = "$expected" ]; then
    pass "$label"
  else
    fail "$label — expected '$expected', got '$got'"
  fi
}

# --- preflight -------------------------------------------------------------

echo "==> Checking prerequisites..."
command -v curl > /dev/null 2>&1 || { echo "curl is required"; exit 1; }
command -v jq   > /dev/null 2>&1 || { echo "jq is required";   exit 1; }

API_KEY="${AGENTMEMORY_E2E_API_KEY:-}"
if [ -z "$API_KEY" ]; then
  echo "AGENTMEMORY_E2E_API_KEY must be set"
  exit 1
fi

echo "==> Checking server health..."
if ! curl -sf "$BASE_URL/health" > /dev/null 2>&1; then
  echo "Server not reachable at $BASE_URL — start it first or set AGENTMEMORY_E2E_BASE_URL"
  exit 1
fi

# --- step 0: create session ------------------------------------------------

echo "--- Step 0: Create test session"
SESSION_RESP=$(curl -sf -X POST "$BASE_URL/v1/api/session/start" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{}')
SESSION_ID=$(echo "$SESSION_RESP" | jq -r '.session_id')
assert_json_has "$SESSION_RESP" '.session_id' "Session created"
echo "       Session ID: $SESSION_ID"

# --- step 1: MCP inject=true -> context_text present -----------------------

echo "--- Step 1: MCP inject=true"

MCP_INJECT_REQ=$(cat <<EOF
{
  "jsonrpc": "2.0",
  "id": "e2e-1",
  "method": "tools/call",
  "params": {
    "name": "memory_observe",
    "arguments": {
      "type": "session_start",
      "title": "E2E MCP inject=true",
      "narrative": "Testing that MCP memory_observe with inject=true returns context_text",
      "session_id": "$SESSION_ID",
      "inject": true
    }
  }
}
EOF
)

MCP_INJECT_RESP=$(curl -s -X POST "$BASE_URL/v1/mcp" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d "$MCP_INJECT_REQ")

# Verify the JSON-RPC result has context_text with non-empty content
assert_json_has "$MCP_INJECT_RESP" '.result | has("context_text")' "MCP inject=true: context_text key present"
# Extract context_text from JSON-RPC text content wrapper
assert_json_nonempty "$MCP_INJECT_RESP" '.result.content[0].text | fromjson | .context_text' "MCP inject=true: context_text non-empty"

# Verify skipped field reflects trigger type
assert_json_has "$MCP_INJECT_RESP" '.result | has("skipped")' "MCP inject=true: skipped present"

# --- step 2: MCP inject=false -> NO context_text ---------------------------

echo "--- Step 2: MCP inject=false"

MCP_NOINJECT_REQ=$(cat <<EOF
{
  "jsonrpc": "2.0",
  "id": "e2e-2",
  "method": "tools/call",
  "params": {
    "name": "memory_observe",
    "arguments": {
      "type": "session_start",
      "title": "E2E MCP inject=false",
      "narrative": "Testing that MCP memory_observe without inject omits context_text",
      "session_id": "$SESSION_ID"
    }
  }
}
EOF
)

MCP_NOINJECT_RESP=$(curl -s -X POST "$BASE_URL/v1/mcp" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d "$MCP_NOINJECT_REQ")

# Verify NO context_text in the result
assert_json_not_has "$MCP_NOINJECT_RESP" '.result | has("context_text")' "MCP inject=false: no context_text"

# --- step 3: REST inject=true -> context_text present ----------------------

echo "--- Step 3: REST inject=true"

REST_INJECT_RESP=$(curl -s -X POST "$BASE_URL/v1/api/observe" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d "{
    \"session_id\": \"$SESSION_ID\",
    \"type\": \"session_start\",
    \"title\": \"E2E REST inject=true\",
    \"narrative\": \"Testing that REST observe with inject=true returns context_text\",
    \"inject\": true
  }")

assert_json_has "$REST_INJECT_RESP" '.context_text' "REST inject=true: context_text key present"
assert_json_nonempty "$REST_INJECT_RESP" '.context_text' "REST inject=true: context_text non-empty"
assert_json_has "$REST_INJECT_RESP" '.skipped' "REST inject=true: skipped present"

# --- step 4: REST inject=false -> NO context_text --------------------------

echo "--- Step 4: REST inject=false"

REST_NOINJECT_RESP=$(curl -s -X POST "$BASE_URL/v1/api/observe" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d "{
    \"session_id\": \"$SESSION_ID\",
    \"type\": \"session_start\",
    \"title\": \"E2E REST inject=false\",
    \"narrative\": \"Testing that REST observe without inject omits context_text\"
  }")

assert_json_not_has "$REST_NOINJECT_RESP" '.context_text' "REST inject=false: no context_text"

# --- step 5: hooks.json validation (T020) ----------------------------------

echo "--- Step 5: Validate hooks.json"

if [ ! -f "$HOOKS_JSON" ]; then
  fail "hooks.json not found at $HOOKS_JSON"
else
  HOOKS_CONTENT=$(cat "$HOOKS_JSON")

  # Assert zero memory_inject_context references (recursive descent catches any nesting)
  INJECT_CTX_COUNT=$(echo "$HOOKS_CONTENT" | jq '[.. | objects | select(has("tool")) | .tool] | map(select(. == "memory_inject_context")) | length')
  assert_eq "$INJECT_CTX_COUNT" "0" "T020: zero memory_inject_context references"

  # Assert every mcp_tool entry has a "server" field
  MISSING_SERVER=$(echo "$HOOKS_CONTENT" | jq '[.. | objects | select(.type == "mcp_tool" and (has("server") | not))] | length')
  assert_eq "$MISSING_SERVER" "0" "T020: every mcp_tool entry has server field"

  # Collect all tool names across all events (recursive descent for any nesting depth)
  TOOL_COUNT=$(echo "$HOOKS_CONTENT" | jq '[.. | objects | select(has("tool")) | .tool] | length')
  assert_eq "$TOOL_COUNT" "14" "T020: exactly 14 hook tool calls"

  # Collect event names
  EVENT_COUNT=$(echo "$HOOKS_CONTENT" | jq '[.hooks | keys[]] | length')
  assert_eq "$EVENT_COUNT" "14" "T020: exactly 14 hook events"

  # Verify all 14 required events are present
  REQUIRED_EVENTS='["SessionStart","UserPromptSubmit","PreToolUse","PostToolUse","PostToolUseFailure","PreCompact","SessionEnd","Notification","Stop","SubagentStart","SubagentStop","TaskCompleted","PostCommit","Diagnostics"]'
  for event in $(echo "$REQUIRED_EVENTS" | jq -r '.[]'); do
    PRESENT=$(echo "$HOOKS_CONTENT" | jq ".hooks | has(\"$event\")")
    if [ "$PRESENT" = "true" ]; then
      pass "T020: hook event '$event' present"
    else
      fail "T020: hook event '$event' missing"
    fi
  done

  # Verify all tools are memory_observe
  NON_OBSERVE_COUNT=$(echo "$HOOKS_CONTENT" | jq '[.. | objects | select(has("tool")) | select(.tool != "memory_observe") | .tool] | unique | length')
  assert_eq "$NON_OBSERVE_COUNT" "0" "T020: all tools are memory_observe"
fi

# --- done -----------------------------------------------------------------

echo ""
echo "All tests completed."
