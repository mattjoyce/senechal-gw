# py-greet

Python example plugin that emits a greeting heartbeat.

## Commands
- `poll` (read): Emit a greeting and update last-run state.
- `health` (read): Return health status.

## Configuration
- `greeting`: Greeting prefix (default: "Hello").
- `name`: Name to greet (default: "World").

## Example
```yaml
plugins:
  py-greet:
    enabled: true
    schedules:
      - every: 1m
    config:
      greeting: "Hi"
      name: "Ductile"
```
