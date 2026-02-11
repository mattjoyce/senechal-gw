---
id: 45
status: done
priority: High
blocked_by: []
tags: [plugin, fabric, ai]
---

# Fabric Plugin - AI Pattern Wrapper

Create a plugin that wraps the `fabric` command-line tool for AI-powered text processing with predefined patterns.

## Overview

Fabric (github.com/danielmiessler/fabric) is an AI tool that applies predefined patterns to text input. This plugin wraps it to make fabric patterns available as Senechal commands.

**Flow:** `input_text | fabric --model <model> --pattern <pattern> | output`

## Use Cases

- Summarize text with `summarize` pattern
- Extract insights with `extract_wisdom` pattern
- Create bullet points with `create_summary` pattern
- Any fabric pattern available on the system

## Implementation

**Plugin Location:** `plugins/fabric/`

**Language:** Python or Bash (Python recommended for better error handling)

**Files:**
```
plugins/fabric/
├── manifest.yaml
├── run.py (or run.sh)
├── requirements.txt (if Python)
└── README.md
```

## Manifest

```yaml
name: fabric
protocol: 1
entrypoint: run.py
description: Wrapper for fabric AI pattern tool

commands:
  - name: execute
    type: write
    description: Execute fabric pattern on input text

  - name: health
    type: read
    description: Check fabric binary is available

config_keys:
  - FABRIC_BIN_PATH      # Path to fabric executable
  - FABRIC_DEFAULT_MODEL # Default model (e.g., gpt-4o-mini)
  - FABRIC_PATTERNS_DIR  # Optional: custom patterns directory
```

## Commands

### `execute` Command

**Input (event payload):**
```json
{
  "text": "Long article text to process...",
  "pattern": "summarize",
  "model": "gpt-4o-mini"  // optional, uses default if omitted
}
```

**Processing:**
1. Read `text` from event payload
2. Use `pattern` from payload (required)
3. Use `model` from payload or config default
4. Execute: `echo "$text" | fabric --model $model --pattern $pattern`
5. Capture stdout as result
6. Handle errors (fabric not found, pattern invalid, API failure)

**Output (response):**
```json
{
  "status": "ok",
  "events": [
    {
      "type": "fabric.completed",
      "payload": {
        "result": "Summarized text output from fabric...",
        "pattern": "summarize",
        "model": "gpt-4o-mini",
        "input_length": 1234,
        "output_length": 456
      }
    }
  ],
  "state_updates": {
    "last_run": "2026-02-10T12:00:00Z",
    "executions_count": 42,
    "last_pattern": "summarize"
  },
  "logs": [
    {"level": "info", "message": "Executed fabric pattern: summarize"}
  ]
}
```

### `health` Command

**Check:**
- Fabric binary exists at configured path
- Fabric executable and has `--help` flag
- Patterns directory accessible (if configured)

**Output:**
```json
{
  "status": "ok",
  "state_updates": {
    "fabric_version": "v1.2.3",
    "available_patterns": 45,
    "last_health_check": "2026-02-10T12:00:00Z"
  }
}
```

## Configuration

**In config.yaml:**
```yaml
plugins:
  fabric:
    enabled: true
    schedule:
      # No schedule - manual/webhook triggered only
    config:
      FABRIC_BIN_PATH: /opt/homebrew/bin/fabric
      FABRIC_DEFAULT_MODEL: gpt-4o-mini
```

## Error Handling

**Handle these errors gracefully:**
1. **Fabric not found** - Check FABRIC_BIN_PATH, return error with setup instructions
2. **Invalid pattern** - List available patterns in error message
3. **API failure** - Fabric's underlying API (OpenAI) fails, return `retry: true`
4. **Empty input** - Validate `text` field exists and non-empty
5. **Timeout** - Fabric hangs, respect job deadline

**Example error response:**
```json
{
  "status": "error",
  "error": "Fabric pattern 'invalid_pattern' not found. Available patterns: summarize, extract_wisdom, create_summary",
  "retry": false,
  "logs": [
    {"level": "error", "message": "Pattern validation failed"}
  ]
}
```

## Testing

### Manual Test (after plugin created)

```bash
# 1. Trigger fabric plugin manually
senechal-gw run fabric execute

# Or via API
curl -X POST http://localhost:8080/trigger/fabric/execute \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "text": "This is a long article about AI and automation...",
    "pattern": "summarize"
  }'

# 2. Check job result
curl http://localhost:8080/job/<job_id> \
  -H "Authorization: Bearer $API_KEY"
```

### Integration Test Scenarios

1. **Happy path:** Valid text + pattern → success
2. **Missing text:** No text field → error
3. **Invalid pattern:** Unknown pattern → error with suggestions
4. **Fabric not installed:** Binary missing → clear setup error
5. **Large input:** 10KB text → handles without issue
6. **Health check:** Returns fabric version and pattern count

## Acceptance Criteria

- ✅ Plugin manifest valid and loads successfully
- ✅ `execute` command processes text through fabric
- ✅ `health` command verifies fabric availability
- ✅ Config keys loaded from config.yaml
- ✅ Errors handled gracefully with helpful messages
- ✅ State updates track execution count and last run
- ✅ Events emitted with fabric output
- ✅ Works with manual trigger (senechal-gw run)
- ✅ Works with API trigger (POST /trigger)
- ✅ All tests pass

## Dependencies

**External:**
- Fabric CLI installed: `brew install fabric` or `go install github.com/danielmiessler/fabric@latest`
- Fabric configured with API keys (OpenAI, etc.)

**Internal:**
- Existing plugin system (Sprint 1)
- API trigger endpoint (Sprint 2)
- Protocol v1 implementation

## Documentation

**Add to USER_GUIDE.md:**
- Section: "Example Plugin: Fabric Wrapper"
- Installation instructions for fabric
- Configuration example
- Usage examples (manual + API)
- Common patterns showcase

## Implementation Notes

**Python implementation (recommended):**
```python
#!/usr/bin/env python3
import json
import subprocess
import sys
from datetime import datetime

def execute_command(config, state, event):
    # Validate input
    text = event.get('payload', {}).get('text')
    if not text:
        return error_response("Missing 'text' field in event payload")

    pattern = event.get('payload', {}).get('pattern')
    if not pattern:
        return error_response("Missing 'pattern' field in event payload")

    model = event.get('payload', {}).get('model', config.get('FABRIC_DEFAULT_MODEL'))

    # Execute fabric
    fabric_bin = config.get('FABRIC_BIN_PATH', 'fabric')
    cmd = [fabric_bin, '--model', model, '--pattern', pattern]

    try:
        result = subprocess.run(
            cmd,
            input=text,
            capture_output=True,
            text=True,
            timeout=60
        )

        if result.returncode != 0:
            return error_response(f"Fabric failed: {result.stderr}")

        return success_response(result.stdout, pattern, model, text)

    except FileNotFoundError:
        return error_response(f"Fabric binary not found at {fabric_bin}")
    except subprocess.TimeoutExpired:
        return error_response("Fabric execution timed out")

def success_response(output, pattern, model, input_text):
    return {
        "status": "ok",
        "events": [{
            "type": "fabric.completed",
            "payload": {
                "result": output,
                "pattern": pattern,
                "model": model,
                "input_length": len(input_text),
                "output_length": len(output)
            }
        }],
        "state_updates": {
            "last_run": datetime.utcnow().isoformat() + "Z",
            "executions_count": (state.get("executions_count", 0) + 1),
            "last_pattern": pattern
        }
    }

def error_response(message):
    return {
        "status": "error",
        "error": message,
        "retry": False
    }

if __name__ == "__main__":
    request = json.load(sys.stdin)
    command = request["command"]
    config = request["config"]
    state = request["state"]
    event = request.get("event", {})

    if command == "execute":
        response = execute_command(config, state, event)
    elif command == "health":
        response = health_check(config)
    else:
        response = error_response(f"Unknown command: {command}")

    print(json.dumps(response))
```

## Estimated Effort

**2-3 hours** (including testing)
- Manifest: 15 min
- Implementation: 60-90 min
- Testing: 30-45 min
- Documentation: 15-30 min

## Future Enhancements (Not in Scope)

- Pattern discovery (list available patterns dynamically)
- Batch processing (multiple texts in one job)
- Custom pattern support (user-defined patterns)
- Streaming output (for long-running patterns)
- Cost tracking (API usage per pattern)

## Narrative

