# Plugin Facts Compliance

This document explains how to make a plugin compatible with the Sprint 7
`plugin_facts` pattern.

The goal is to move durable plugin truth toward **append-only facts** while
keeping `plugin_state` as a compatibility/current-state row for existing code
paths.

## 1. What Changes For A Plugin

Before Sprint 7, a plugin typically returned `state_updates` and core
shallow-merged them directly into `plugin_state`.

With `plugin_facts`, a plugin can instead participate in a stricter pattern:

1. The plugin emits a successful, stable snapshot in `state_updates`.
2. Core records that snapshot as an append-only row in `plugin_facts`.
3. Core derives the compatibility `plugin_state` row from the newest fact.

This means:
- `plugin_facts` is the historical truth.
- `plugin_state` is the compatibility cache/current view.

## 2. Current Migrated Plugins

Sprint 7 started with `file_watch` as the exemplar, and the current branch now
applies the pattern to the shipped plugins that still emitted durable
`state_updates`.

Current fact contracts:
- `file_watch` `poll` -> `file_watch.snapshot`
- `folder_watch` `poll` -> `folder_watch.snapshot`
- `py-greet` `poll` -> `py-greet.snapshot`
- `ts-bun-greet` `poll` -> `ts-bun-greet.snapshot`
- `stress` `state` -> `stress.state_snapshot`

Watcher `health` commands are intentionally **not** part of this durable fact
flow. Health is diagnostic and should not mutate durable state.

## 3. Compliance Rules

If you want a plugin to be compatible with this pattern, the plugin and core
need a clear, defensible contract.

### Plugin-side rules

- Emit facts only from commands that produce meaningful durable truth.
- Prefer successful `poll` or equivalent snapshot-producing commands.
- Do not use `health` as durable state.
- Keep the emitted snapshot shape stable and explicit.
- Return a full snapshot, not a partial patch, when the fact is intended to
  represent current durable truth.
- Keep the snapshot JSON object-shaped and deterministic enough for operators to
  inspect.

### Core-side rules

- Assign an explicit fact type.
- Record each fact append-only in `plugin_facts`.
- Define how compatibility `plugin_state` is derived from that fact type.
- Add an operator-visible read path.
- Add tests that prove both fact persistence and derived compatibility state.

## 4. Recommended Fact Shape

Use a fact when the plugin can answer:

> "What is the current durable observed state of this plugin right now?"

Good candidates:
- watcher snapshots
- cursors/checkpoints
- discovered remote resource inventories
- reducer-friendly state snapshots

Poor candidates:
- transient health checks
- ephemeral timing/latency noise
- values that are meaningful only to a single in-flight job

For a first migration, prefer a **full snapshot** over incremental diffs.

## 5. Snapshot Examples

### `file_watch`

`file_watch poll` returns a snapshot shaped like:

```json
{
  "watches": {
    "single-file": {
      "exists": true,
      "fingerprint": "abc123",
      "size": 42,
      "mtime_ns": 1713740000000000000,
      "path": "/tmp/file.txt",
      "strategy": "sha256",
      "updated_at": "2026-04-22T01:02:03Z"
    }
  },
  "last_poll_at": "2026-04-22T01:02:03Z"
}
```

Core then:
- stores that JSON in `plugin_facts.fact_json`
- tags it `file_watch.snapshot`
- updates `plugin_state` for `file_watch` to the same snapshot shape

This keeps legacy state readers working while giving operators an append-only
history.

### `folder_watch`

`folder_watch poll` returns the same top-level compatibility shape:

```json
{
  "watches": {
    "docs": {
      "root": "/srv/content",
      "files": {
        "summary.md": "abc123"
      },
      "snapshot_hash": "def456",
      "file_count": 1,
      "updated_at": "2026-04-22T01:02:03Z"
    }
  },
  "last_poll_at": "2026-04-22T01:02:03Z"
}
```

### `py-greet` and `ts-bun-greet`

The example greeting plugins emit a tiny full snapshot:

```json
{
  "last_run": "2026-04-22T01:02:03Z",
  "last_greeting": "Hello, Ductile!"
}
```

### `stress`

The `stress state` command emits the full compatibility snapshot for its only
durable datum:

```json
{
  "count": 42
}
```

## 6. Migration Checklist For Another Plugin

When migrating another plugin to `plugin_facts`, do all of the following:

1. Choose one command that produces durable truth.
2. Define one explicit fact type.
3. Make the plugin emit a stable object snapshot.
4. Ensure the snapshot is a full compatibility-state view, not just a partial
   patch.
5. Update core to recognize that plugin+command and record the fact.
6. Add or update the reducer that derives `plugin_state`.
7. Add operator inspection support.
8. Add unit tests for persistence and derived state.
9. Add a Docker or similarly realistic fixture when runtime behavior matters.
10. Document the fact type, snapshot shape, and non-goals.

## 7. Questions To Resolve Before Migrating A Plugin

Before making a plugin compliant, answer these questions:

- What exact command owns durable truth?
- Is the emitted JSON a full snapshot or only a delta?
- Should `plugin_state` mirror the newest fact exactly, or should a reducer
  transform it?
- What data should remain diagnostic only and stay out of durable storage?
- How will an operator inspect recent facts?
- What realistic test proves the fact path end to end?

If those answers are vague, the plugin is not ready for `plugin_facts` yet.

## 8. Deployment Note

For existing databases, apply the Sprint 7 schema migration before a normal
deploy:

```bash
python3 scripts/migrate-hickey-sprint-7-plugin-facts.py /path/to/ductile.db
```

Then restart or deploy the updated binary.

For non-empty existing databases, startup should validate and fail if the
required Sprint 7 schema is missing. It should not silently add `plugin_facts`
or related indexes during normal open.
