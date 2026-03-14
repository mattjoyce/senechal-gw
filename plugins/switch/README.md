# switch

Classifier plugin that routes payloads by field value and emits the first matching event type.

## Commands
- `handle` (write): Evaluate configured checks and emit the first matching event type.
- `health` (read): Validate configuration.

## Configuration
Required:
- `field`: Field path to inspect (e.g. `payload.text`).
- `checks`: Ordered list of checks with an `emit` event type.

Supported check types: `contains`, `startswith`, `endswith`, `equals`, `regex`, `default`.

## Example
```yaml
plugins:
  switch:
    enabled: true
    config:
      field: payload.text
      checks:
        - contains: "error"
          emit: alerts.error
        - default: alerts.ok
```
