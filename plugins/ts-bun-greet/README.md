# ts-bun-greet

TypeScript/Bun example plugin that emits a greeting heartbeat.

## Commands
- `poll` (read): Emit a greeting and produce the current greeting snapshot.
- `health` (read): Return health status.

## Configuration
- `greeting`: Greeting prefix (default: "Hello").
- `name`: Name to greet (default: "World").

## Persistence
Successful `poll` runs emit a snapshot state shaped as:
- `last_run`
- `last_greeting`

Core records that snapshot as append-only `plugin_facts` rows with fact type `ts-bun-greet.snapshot` and keeps `plugin_state` as the latest compatibility snapshot.

## Example
```yaml
plugins:
  ts-bun-greet:
    enabled: true
    schedules:
      - every: 1m
    config:
      greeting: "Hi"
      name: "Ductile"
```
