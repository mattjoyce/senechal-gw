---
id: 88
status: todo
priority: Medium
tags: [docs, api, ux, testing]
---

# DOC: API trigger endpoint payload structure undocumented

## Description

The `/trigger/{plugin}/{command}` endpoint requires payload to be wrapped in a `{"payload": {...}}` object, but this isn't documented. Easy to get wrong on first use.

## Impact

- **Severity**: Medium - Causes confusion but workaround is simple
- **Scope**: API users, testing, integration
- **User Experience**: Trial and error to discover correct format

## Evidence

**Test**: TP-002 (2026-02-13)

**Initial attempt** (failed):
```bash
curl -X POST /trigger/file_handler/handle \
  -d '{"action": "read", "file_path": "..."}'
# Plugin received empty action
```

**Correct format** (works):
```bash
curl -X POST /trigger/file_handler/handle \
  -d '{"payload": {"action": "read", "file_path": "..."}}'
# Plugin received action="read" correctly
```

## Root Cause

**Code**: `/home/matt/senechal-gw/internal/api/types.go`
```go
type TriggerRequest struct {
    Payload json.RawMessage `json:"payload,omitempty"`
}
```

**Documentation gap**:
- USER_GUIDE.md doesn't cover API trigger usage
- No API documentation file
- Type definition not referenced in docs

## Recommendation

### 1. Add to USER_GUIDE.md

New section: "API Manual Triggering"

```markdown
### Manual Plugin Execution

Trigger a plugin command via the API:

**Endpoint**: `POST /trigger/{plugin}/{command}`

**Headers**:
- `Authorization: Bearer <token>`
- `Content-Type: application/json`

**Body** (wrap payload in `payload` field):
```json
{
  "payload": {
    "your": "plugin-specific",
    "data": "here"
  }
}
```

**Example** (trigger file read):
```bash
curl -X POST http://localhost:8080/trigger/file_handler/handle \
  -H "Authorization: Bearer test_token" \
  -H "Content-Type: application/json" \
  -d '{
    "payload": {
      "action": "read",
      "file_path": "/path/to/file.md"
    }
  }'
```

**Response**:
```json
{
  "job_id": "uuid",
  "status": "queued",
  "plugin": "file_handler",
  "command": "handle"
}
```
```

### 2. Create API_REFERENCE.md

Document all API endpoints:
- `/trigger/{plugin}/{command}` - Manual execution
- Authentication headers
- Request/response schemas
- Error codes

### 3. Add examples to CLI help

```bash
$ senechal-gw help trigger
# Should show API curl examples
```

## Testing Impact

During TP-002, spent 10+ minutes debugging why plugin received empty payload. Clear docs would have prevented this.

## Narrative

- 2026-02-13: Discovered during TP-002 API trigger testing. First attempt sent raw payload JSON, but plugin received empty action field. After checking code (types.go), found TriggerRequest wraps payload. Second attempt with wrapped payload succeeded. This is not intuitive and wastes time during testing/integration. Recommend adding clear API documentation with examples. (by @test-admin)
