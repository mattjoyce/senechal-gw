---
id: 108
status: backlog
priority: Medium
blocked_by: []
tags: [api, plugins, schema, rfc-004]
---

# Improvement: Compact Schema Format Should Support Required Fields

## Problem

The compact schema format (`field: type`) introduced in feat/api-plugin-discovery cannot
express required fields. When a plugin author (or LLM) uses compact format, all parameters
appear optional to API consumers.

**Example manifest:**
```yaml
input_schema:
  url: string
  language: string
```

**Expanded output via GET /plugin/{name}:**
```json
{
  "type": "object",
  "properties": {
    "url": { "type": "string" },
    "language": { "type": "string" }
  }
}
```

Note: `required` key is absent. An LLM reading this schema cannot distinguish `url`
(mandatory) from `language` (optional).

**Contrast with full JSON Schema format (youtube_transcript):**
```json
{
  "type": "object",
  "properties": { ... },
  "required": ["url"]
}
```

## Why It Matters

This is directly relevant to RFC-004 (LLM as Operator). An LLM authoring a plugin manifest
should be able to write concise YAML and have required fields correctly expressed to downstream
consumers (e.g. AgenticLoop's DuctileTool). Without required info, the LLM agent using the
plugin may treat all parameters as optional and omit mandatory ones.

## Proposed Solutions

### Option A: Bang suffix convention
```yaml
input_schema:
  url!: string      # required
  language: string  # optional
```
Expands `url!` → strip `!`, add to `required` array.

### Option B: Separate `_required` key
```yaml
input_schema:
  _required: [url]
  url: string
  language: string
```
Conventional — less ergonomic but no ambiguity in field names.

### Option C: Type suffix with `!`
```yaml
input_schema:
  url: string!     # value side marks required
  language: string
```

### Option D: Accept the limitation, document it
Compact format is for simple cases where all fields are optional. If any required field
exists, the author must use full JSON Schema. Document this clearly.

## Recommendation

Option A (bang suffix on key) is concise and readable for LLM authoring. Option D is the
lowest-effort fallback and avoids complicating the expansion logic.

## Test Cases

- `url!: string` → `properties.url.type = string`, `required` contains `url`
- `url: string` → `properties.url.type = string`, `required` does NOT contain `url`
- Mix of required and optional fields
- Interaction with full JSON Schema passthrough (should be unaffected)

## References

- feat/api-plugin-discovery (merged 2026-02-18)
- RFC-004: LLM as Operator/Admin
- Tested: compact format confirmed working, required gap confirmed missing (2026-02-18)
