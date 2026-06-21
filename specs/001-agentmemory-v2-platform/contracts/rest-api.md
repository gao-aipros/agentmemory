# REST API Contracts: AgentMemory v2

**Feature**: AgentMemory v2 Platform Migration
**Date**: 2026-06-21
**Source**: `docs/specs/09-routing.md`, `docs/specs/08-hooks.md`

## Protocol

- **Base path**: `/v1/api/`
- **Auth**: `Authorization: Bearer st_<jwt>` or `Bearer ak_<api_key>`
- **Content-Type**: `application/json`
- **Port**: Single HTTP port (all traffic)

## Endpoints

### Observation

```
POST /v1/api/observe
```
**Auth**: `st_` or `ak_`
**Body**:
```json
{
  "type": "tool_use",
  "title": "Read files/file.go",
  "narrative": "Agent read file.go to understand routing logic",
  "facts": "file.go defines chi router with 6 route groups",
  "concepts": ["routing", "chi", "middleware"],
  "files": ["internal/handler/rest.go"],
  "importance": 0.6,
  "session_id": "sess_abc123"
}
```
**Response** (200):
```json
{
  "observation_id": "obs_xyz789",
  "status": "recorded"
}
```
**Response** (400): `{"error": "missing required field: type"}`
**Response** (401): `{"error": "invalid or expired token"}`

### Session End

```
POST /v1/api/session/end
```
**Auth**: `st_` or `ak_`
**Body**:
```json
{
  "session_id": "sess_abc123"
}
```
**Response** (200):
```json
{
  "session_id": "sess_abc123",
  "status": "ended",
  "summary_queued": true,
  "consolidation_queued": true
}
```

### Session Commit

```
POST /v1/api/session/commit
```
**Auth**: `st_` or `ak_`
**Body**:
```json
{
  "session_id": "sess_abc123",
  "sha": "ccef66b",
  "branch": "main",
  "message": "docs: refine constitution"
}
```
**Response** (200):
```json
{
  "linked": true,
  "commit_sha": "ccef66b",
  "session_id": "sess_abc123"
}
```

### Auth: Login

```
POST /v1/auth/login
```
**Auth**: None
**Body**:
```json
{
  "email": "user@example.com",
  "password": "secure-password",
  "totp_code": "123456"
}
```
**Response** (200):
```json
{
  "token": "st_eyJhbGciOi...",
  "user": {
    "id": "usr_123",
    "email": "user@example.com",
    "name": "Alice"
  },
  "expires_at": "2026-06-22T00:00:00Z"
}
```
**Response** (401): `{"error": "invalid credentials"}`
**Response** (401, TOTP): `{"error": "invalid TOTP code"}`

### Auth: Manage API Keys

```
GET    /v1/auth/keys          # List own keys
POST   /v1/auth/keys          # Create key
DELETE /v1/auth/keys/{key_id} # Revoke key
```
**Auth**: `st_` (required; `ak_` rejected)
**POST Body**:
```json
{
  "label": "Claude Code dev",
  "expires_at": "2026-12-31T00:00:00Z"
}
```
**POST Response** (201):
```json
{
  "key_id": "key_456",
  "key": "ak_full_key_value_once",
  "prefix": "ak_abc12345",
  "label": "Claude Code dev",
  "expires_at": "2026-12-31T00:00:00Z"
}
```

### Health

```
GET /health
```
**Auth**: None
**Response** (200): `{"status": "ok", "db": "connected"}`
**Response** (503): `{"status": "unhealthy", "db": "disconnected"}` or `{"status": "unhealthy", "migrations": "pending"}`

### WebSocket

```
GET /v1/socket
```
**Auth**: `st_` (required; `ak_` rejected)
**Protocol**: WebSocket upgrade. Server pushes session observation events.
Follows v1 viewer protocol behavior.

## Hook → REST Mapping

Each hook event maps to a REST endpoint:

| Hook | REST Call | Method |
|------|-----------|--------|
| SessionStart | `/v1/api/observe` + `?inject_context=true` | POST |
| UserPromptSubmit | `/v1/api/observe` | POST |
| PreToolUse | `/v1/api/observe` + `?inject_context=true` | POST |
| PostToolUse | `/v1/api/observe` | POST |
| PostToolUseFailure | `/v1/api/observe` | POST |
| PreCompact | `/v1/api/observe` + `?inject_context=true` | POST |
| SubagentStart | `/v1/api/observe` | POST |
| SubagentStop | `/v1/api/observe` | POST |
| Notification | `/v1/api/observe` (filter: permission allow only) | POST |
| TaskCompleted | `/v1/api/observe` | POST |
| PostCommit | `/v1/api/session/commit` | POST |
| SessionEnd | `/v1/api/session/end` | POST |
| PermissionPrompt | `/v1/api/observe` | POST |

## Error Responses

All error responses follow the format:
```json
{
  "error": "human-readable message",
  "code": "ERROR_CODE"
}
```

| HTTP Status | Code | Meaning |
|-------------|------|---------|
| 400 | `BAD_REQUEST` | Missing/invalid field |
| 401 | `UNAUTHORIZED` | Missing/expired/invalid token |
| 403 | `FORBIDDEN` | Valid token but insufficient scope |
| 404 | `NOT_FOUND` | Resource not found |
| 409 | `CONFLICT` | Duplicate or conflicting state |
| 429 | `RATE_LIMITED` | Too many requests |
| 500 | `INTERNAL_ERROR` | Server error (retry with backoff) |
| 503 | `SERVICE_UNAVAILABLE` | DB down or migrations pending |
