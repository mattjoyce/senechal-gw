# improvement-110: agenticloop config base_dir for path resolution

## Status
backlog

## Context
All relative paths in `config.yaml` (e.g. `./data/agenticloop.db`,
`./data/workspaces`) are resolved against the process CWD, not the config
file's location. This means the service must be launched from a specific
directory to work correctly, which is fragile and non-obvious.

Discovered during testing: service launched from `/home/matt/admin` with
config at `AgenticLoop-test/config.yaml` — data ended up in
`/home/matt/admin/data/` not `AgenticLoop-test/data/` as expected.

## Proposal
Add an optional `base_dir` field to config (top-level or under `storage`):

```yaml
base_dir: "/home/matt/admin/AgenticLoop-test"   # all relative paths resolve from here
storage:
  path: ./data/agenticloop.db
  workspace_dir: ./data/workspaces
```

If `base_dir` is not set, fall back to current behaviour (CWD).
`base_dir` itself should support `~` expansion and env var interpolation.

## Behaviour
- All relative paths in config resolved against `base_dir`
- `base_dir` can be absolute or relative (relative = relative to CWD, as now)
- Makes service launchable from any directory
