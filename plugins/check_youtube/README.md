# check_youtube

Classifier plugin that routes payloads based on a field value. This is a named instance of the `switch` classifier, preconfigured for URL detection (e.g., YouTube vs web).

## Commands
- `handle` (write): Evaluate configured checks and emit the first matching event type.
- `health` (read): Validate configuration.

## Configuration
Required:
- `field`: Field path to inspect (e.g. `payload.url`, `payload.text`).
- `checks`: Ordered list of checks with an `emit` event type.

Supported check types: `contains`, `startswith`, `endswith`, `equals`, `regex`, `default`.

## Example
```yaml
plugins:
  check_youtube:
    enabled: true
    config:
      field: payload.url
      checks:
        - contains: "youtu.be"
          emit: youtube.url.detected
        - contains: "youtube.com"
          emit: youtube.url.detected
        - default: web.url.detected
```
