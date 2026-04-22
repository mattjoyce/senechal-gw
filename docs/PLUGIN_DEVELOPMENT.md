# Plugin Development Guide

Ductile is built on a **spawn-per-command** model. Any executable that can read JSON from `stdin` and write JSON to `stdout` can serve as a Ductile plugin (or "Skill").

See also: [Plugin Facts Compliance](./PLUGIN_FACTS.md) for the Sprint 7
append-only fact pattern and the current `file_watch` exemplar.

---

## 1. The Lifecycle

When a job is triggered (via scheduler, API, or webhook):
1.  Ductile forks the plugin entrypoint.
2.  The core writes a **Request Envelope** to the plugin's `stdin`.
3.  The plugin processes the command and writes a **Response Envelope** to `stdout`.
4.  Ductile captures `stderr` for logging and kills the process if it exceeds the timeout.

---

## 2. Protocol v2

### Request Envelope (Core → Plugin)
```json
{
  "protocol": 2,
  "job_id": "uuid",
  "command": "poll | handle | health | init",
  "config": {},
  "state": {},
  "context": {},
  "workspace_dir": "/path/to/workspace",
  "event": {},
  "deadline_at": "ISO8601"
}
```
-   `context`: Shared metadata ("Baggage") carried across the pipeline chain.
-   `workspace_dir`: A dedicated local directory for this job's artifacts.

### Response Envelope (Plugin → Core)
```json
{
  "status": "ok | error",
  "result": "short human-readable summary",
  "error": "message",
  "retry": true,
  "events": [],
  "state_updates": {},
  "logs": []
}
```
-   `result`: **Required for `status: ok`**. A short human-readable summary of what the plugin did.
-   `retry`: Protocol v2 compatibility signal for permanent failures. Omit it for the default retryable error path; set `false` only when retrying the same request cannot succeed. Core owns the final retry decision.
-   `state_updates`: Top-level keys here are shallow-merged into the plugin's persistent state.
-   Some shipped plugins may also have specific successful `state_updates` promoted by core into append-only `plugin_facts` rows. Sprint 7 starts this with `file_watch poll` snapshots while keeping protocol v2 unchanged.
-   `events`: An array of `{ "type": "...", "payload": {} }` to trigger downstream pipelines.

For a plugin to participate in `plugin_facts`, the emitted `state_updates`
should be a stable object snapshot with an explicit reducer/compatibility story,
not just arbitrary mutable metadata. `health` commands should remain diagnostic
unless there is a very strong reason otherwise.

---

## 3. Bash Example

```bash
#!/usr/bin/env bash
set -euo pipefail

# Read the request
REQUEST=$(cat)

# Extract fields using jq
COMMAND=$(echo "$REQUEST" | jq -r '.command')
MESSAGE=$(echo "$REQUEST" | jq -r '.config.message // "Hello"')

# Write a log file to the workspace
WORKSPACE=$(echo "$REQUEST" | jq -r '.workspace_dir')
echo "$MESSAGE" > "$WORKSPACE/output.txt"

# Respond
cat <<EOF
{
  "status": "ok",
  "result": "Command $COMMAND executed successfully",
  "logs": [{"level": "info", "message": "Command $COMMAND executed successfully"}]
}
EOF
```

---

## 4. Python Example

Python plugins should use **uv** (installed in the ductile runtime image) with [PEP 723 inline script metadata](https://peps.python.org/pep-0723/). Declare dependencies at the top of the script.

Important runtime detail: Ductile executes the manifest `entrypoint` directly. It does not wrap Python entrypoints with `uv` automatically. Use a uv shebang (or wrapper script) in the entrypoint.

```python
#!/usr/bin/env -S uv run --script
# /// script
# dependencies = [
#   "requests>=2.31",
# ]
# ///

import sys, json, os

def main():
    # Read request
    req = json.load(sys.stdin)
    command = req.get("command")
    config = req.get("config", {})

    # Process
    if command == "poll":
        # ... logic ...
        pass

    # Build response
    resp = {
        "status": "ok",
        "result": "Python plugin active",
        "state_updates": {"last_seen": "now"},
        "logs": [{"level": "info", "message": "Python plugin active"}]
    }

    # Write response
    json.dump(resp, sys.stdout)

if __name__ == "__main__":
    main()
```

**Plugin directory structure (Python):**
```
plugins/my-plugin/
├── manifest.yaml    # entrypoint: run.py
└── run.py           # executable + PEP 723 inline deps
```

Ensure the entrypoint is executable:
```bash
chmod +x plugins/my-plugin/run.py
```

---

## 5. Event Payload Convention

Ductile plugins should follow standard payload field conventions for interoperability:

### Standard Fields

| Field | Type | Purpose | Used By |
|-------|------|---------|---------|
| `text` | string | Primary text content for processing | **Required** if producing text for downstream steps |
| `result` | string | Final human-readable output | Terminal plugins (fabric, summarizers) |
| `source_url` | string | Originating URL | Web scrapers, YouTube fetchers |
| `source_type` | string | Content origin hint | All plugins |

### Source Types

- `web` - Web page content (jina-reader)
- `youtube` - YouTube video transcript
- `file` - Local file content
- `llm` - LLM-generated content (fabric, claude, etc.)

### Event Type Naming

Event types should follow the pattern: `<plugin_name>.<past_tense_verb>`

Examples:
- `jina_reader.scraped` - jina-reader completed scraping
- `youtube_transcript.fetched` - youtube_transcript fetched transcript
- `fabric.completed` - fabric completed processing
- `file_handler.read` - file_handler read file
- `file_handler.written` - file_handler wrote file

### Pipeline Integration

**Context Auto-Propagation**: The core dispatcher automatically propagates these fields from input events to output events:
- `pattern` - Fabric pattern name
- `prompt` - User prompt
- `model` - LLM model name
- `output_dir` - Output directory path
- `output_path` - Full output file path
- `filename` - Output filename

Plugins **do not** need to manually copy these fields. The following code is **unnecessary**:

```python
# OLD (no longer needed):
for field in ["pattern", "prompt", "model", "output_dir", "output_path", "filename"]:
    if field in payload:
        out_payload[field] = payload[field]
```

Simply emit your event with standard fields (`text`, `result`, `source_url`, `source_type`) and the dispatcher handles propagation.

### Baggage (Context) Fallback

Every event payload is shallow-merged into the persistent **context** ledger. Downstream plugins receive this accumulated baggage in `request.context`. If a field may be produced by an upstream step, prefer:

1. Read from `event.payload` (step-specific input).
2. Fall back to `request.context` for accumulated values.

This makes pipelines resilient when intermediate plugins emit narrower payloads.

### Example Event Payload

```python
return {
    "status": "ok",
    "result": "Scraped https://example.com",
    "events": [{
        "type": "jina_reader.scraped",
        "payload": {
            "url": "https://example.com",
            "source_url": "https://example.com",
            "source_type": "web",
            "text": "Scraped content here...",  # For downstream processors
            "content_hash": "abc123"
        }
    }]
}
```

---

## 6. The Manifest (`manifest.yaml`)

Every plugin must have a `manifest.yaml` in its directory.

```yaml
manifest_spec: ductile.plugin
manifest_version: 1
name: echo
version: 0.1.0
protocol: 2
entrypoint: run.sh
description: "A demonstration plugin that echoes input." # Used by LLM operators
concurrency_safe: true # Optional; default true. Set false for functional-state plugins.
commands:
  - name: poll
    type: write
    description: "Emits echo.poll events." # Critical for TUI/LLM clarity
    input_schema:
      message: string
    output_schema:
      status: string
  - name: health
    type: read
    description: "Returns plugin version."
```

### Manifest Fields
- `manifest_spec`: Required manifest schema identifier. Current supported value: `ductile.plugin`.
- `manifest_version`: Required manifest schema version. Current supported value: `1`.
- `description`: A human-readable (and LLM-readable) summary of what the plugin or command does.
- `concurrency_safe`: Optional boolean concurrency hint. Default is `true`. Set `false` for plugins whose correctness depends on serialized execution (e.g. functional state snapshots). Runtime behavior: `false` plugins run serial by default unless operator explicitly overrides `plugins.<name>.parallelism > 1` in config.
- `type`: `read` (no side effects) or `write` (mutates state or external systems). This determines the token scope required to invoke it.
- `input_schema` / `output_schema`: (Optional) legacy JSON Schema describing the command's expected payload and result.
- `values`: (Optional) names-only payload contract for authoring pipelines. `values.consume` declares request payload names the command consumes, and `values.emit` declares event payload names the command emits. It does not declare types and does not make anything durable.

#### Compact Schema Format
To keep manifests concise, you can use a compact map of `field: type` instead of a full JSON Schema object. Ductile will automatically expand this into a complete JSON Schema for API consumers.

Example compact input:
```yaml
input_schema:
  url: string
  depth: integer
```

Expands to:
```json
{
  "type": "object",
  "properties": {
    "url": { "type": "string" },
    "depth": { "type": "integer" }
  }
}
```

If you need more control (descriptions, constraints), you can provide a full JSON Schema object instead.

#### Names-Only Value Contracts
For Sprint 3 explicit durability, plugin manifests can declare consumed and
emitted value names without committing to a full type schema. This is a sanity
aid for authors: `values.consume` tells them what request names a plugin
expects, and `values.emit` tells them what event names the plugin may emit.
`input_schema` remains as the legacy typed/schema surface during the transition.

```yaml
commands:
  - name: handle
    type: write
    values:
      consume:
        - payload.url
        - payload.message
      emit:
        - event: content_ready
          values:
            - payload.url
            - payload.content
            - payload.content_hash
            - payload.truncated
```

Rules:

- `values.consume` entries are request payload names, such as `payload.url`.
- `values.emit[].values` entries are emitted event payload names, such as `payload.content_hash`.
- Entries are names only. Do not infer JSON types from them.
- Values do not decide durability. Pipeline authors still decide durable names with `baggage:`.
- Values are local plugin contracts. Authors use `with:` to transform durable context into the request payload a downstream plugin expects.

---

## 7. Built-in Plugin: `if` Classifier

The `if` plugin is a general-purpose field classifier. It evaluates an ordered list of checks against a payload field and emits the **first** matching event type with the payload unchanged.

### Config (per instance)

```yaml
plugins:
  check_youtube:
    enabled: true
    config:
      field: text
      checks:
        - contains: "youtu.be"
          emit: youtube.url.detected
        - contains: "youtube.com"
          emit: youtube.url.detected
        - startswith: "http"
          emit: web.url.detected
        - default: text.received
```

### Supported checks (v1)

- `contains`, `startswith`, `endswith`, `equals` (case-insensitive)
- `regex` (Python `re.fullmatch` against the field value)
- `default` (always matches if reached)

### Semantics

- Checks are evaluated in order; first match wins.
- Missing fields are treated as empty strings.
- No match + no default → `status: error` with `retry: false`. Core treats this as a v2 compatibility signal for a permanent failure.

### Instance naming

Ductile currently uses manifest names as plugin names. To create multiple instances, copy the plugin directory and give each manifest a unique `name` (for example `check_youtube`).

---

## 8. Security & Isolation

-   **Allowed Paths:** Plugins should only read/write within their provided `workspace_dir` or explicitly configured paths.
-   **Execution:** Plugins run as the same OS user as the gateway. Use filesystem permissions to limit their scope.
-   **Trust:** Ductile will refuse to load plugins with world-writable directories or path traversal attempts in their entrypoints.
