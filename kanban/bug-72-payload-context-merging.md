---
id: 72
status: done
priority: Normal
tags: [bug, plugin, pipeline, payload, protocol]
---

# BUG: Plugins need to merge event payload with context (baggage) fields

## Description

In multi-hop pipelines, plugins receive data in two places:
1. `event.payload` - immediate event data from the previous step
2. `context` - accumulated baggage from all previous steps

Currently, plugins only check `event.payload` for fields, causing failures when downstream steps need data from earlier in the chain.

## Impact

- **Severity**: Normal - Workaround exists (include all fields in every event)
- **Scope**: All plugins that need cross-hop data propagation
- **User Experience**: Pipelines fail at later steps with "missing field" errors

## Evidence

E2E test of file_handler → fabric → file_handler pipeline:

1. **Initial trigger** includes `output_dir`: `/home/matt/admin/senechal-test/reports`
2. **Baggage accumulation works**: `output_dir` correctly stored in event_context table
3. **Final step fails**: file_handler write action reports "Missing required field 'output_dir' in payload"

**Root cause**: file_handler plugin only checks `payload.get("output_dir")` but doesn't check `context.get("output_dir")`

## Database Evidence

```sql
-- Event context shows output_dir is preserved:
SELECT step_id, json_extract(accumulated_json, '$.output_dir')
FROM event_context
WHERE pipeline_name = 'file-to-report';

-- Result:
-- analyze  | /home/matt/admin/senechal-test/reports
-- save     | /home/matt/admin/senechal-test/reports
```

## Expected Behavior

Plugins should check for fields in this order:
1. First check `event.payload` (immediate data)
2. If not found, check `context` (baggage)
3. This allows data to flow through multi-hop chains

## Reproduction

```bash
# Start gateway locally
cd ~/admin/senechal-test
./senechal-gw system start --config config.yaml &

# Trigger 3-hop pipeline
curl -X POST http://localhost:8080/trigger/file_handler/handle \
  -H "Authorization: Bearer test_admin_token_local" \
  -d '{
    "payload": {
      "action": "read",
      "file_path": "/home/matt/admin/senechal-test/test-files/sample.md",
      "pattern": "summarize",
      "output_dir": "/home/matt/admin/senechal-test/reports"
    }
  }'

# Observe: fabric job succeeds, but final file_handler write fails
```

## Suggested Fix

Update plugin protocol helper or best practices:

```python
def get_field(field_name, event, context):
    """Get field from event payload, falling back to context."""
    return event.get("payload", {}).get(field_name) or context.get(field_name)
```

Plugins should use this pattern for fields that may come from earlier pipeline steps.

## Workaround

Currently, each plugin must explicitly forward all fields that downstream steps might need. Example: file_handler emits both `content` and `pattern` so fabric receives them both.

## Related

- Card #62: Multi-Plugin Pipeline E2E Test (discovered this issue)
- ROUTING_SPEC_GEMINI.md: Defines Protocol v2 with `context` field
- PIPELINES.md: Documents baggage accumulation

## Narrative

- 2026-02-12: Discovered during E2E pipeline testing for card #62. The 3-hop pipeline (file_handler → fabric → file_handler) successfully routes events and accumulates baggage in event_context, but the final plugin doesn't access the context field. Verified that `output_dir` is preserved in the database's accumulated_json but not accessible to the plugin. This reveals a pattern: plugins need guidance or helper functions to merge payload + context fields. (by @test-admin)
