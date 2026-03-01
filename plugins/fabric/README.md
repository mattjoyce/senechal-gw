# fabric

Wrapper around the `fabric` CLI tool for text processing and summarization.

## Commands
- `poll` (write): No-op scheduled tick.
- `handle` (write): Process text/URL/YouTube input through a fabric pattern.
- `health` (read): Validate the fabric binary and list patterns.

## Configuration
Optional:
- `FABRIC_BIN_PATH`: Path to `fabric` binary (default: `fabric`).
- `FABRIC_DEFAULT_PATTERN`: Default pattern when none supplied.
- `FABRIC_DEFAULT_PROMPT`: Default prompt.
- `FABRIC_DEFAULT_MODEL`: Default model.

## Input (handle)
Payload fields: `text`, `url`, `youtube_url`, `pattern`, `prompt`, `model`.

## Events
Emits `fabric.completed` with payload containing `result`, `pattern`, `prompt`, `model`, and length metadata.

## Example
```yaml
plugins:
  fabric:
    enabled: true
    config:
      FABRIC_DEFAULT_PATTERN: summarize
      FABRIC_DEFAULT_MODEL: gpt-4o
```
