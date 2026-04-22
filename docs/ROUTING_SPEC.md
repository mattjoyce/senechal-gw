# Ductile — Routing & Orchestration Specification

**Version:** 1.0 (Gemini Consensus)  
**Date:** 2026-02-11  
**Model:** Governance Hybrid (DB + Workspace)

---

## 1. Overview

Ductile uses a **Graph-based Pipeline** model to orchestrate event flow. It separates **Governance** (metadata/context) from **Execution** (artifacts/workspace).

### 1.1 Core Components
*   **Control Plane (DB):** A SQLite ledger (`event_context`) that accumulates metadata ("Baggage") across hops.
*   **Data Plane (Filesystem):** A job-specific directory (`workspace_dir`) that holds large binary artifacts (audio, images, docs).
*   **Orchestrator (DSL):** A YAML-based Pipeline DSL that supports nesting, branching, and single-root triggers.

---

## 2. Pipeline DSL

Pipelines are defined in YAML files referenced via `include:` in `config.yaml` (files or directories).

### 2.1 Syntax

```yaml
pipelines:
  - name: wisdom-chain
    on: discord.video_link_received   # The "Single Root" trigger
    steps:
      - id: downloader
        uses: yt-dlp-plugin
      
      - id: processing
        call: standard-audio-wisdom   # Nested Pipeline call
      
      - id: delivery
        split:                        # Branching logic
          - uses: discord-notifier
          - steps:                    # Sequential branch
              - uses: s3-archiver
              - uses: db-indexer
```

### 2.2 Functional Blocks
*   **uses:** Execute a specific plugin command.
*   **call:** Execute another named pipeline (reusable middleware).
*   **split:** Branch execution into multiple parallel paths.
*   **on:** The event that triggers the root of the pipeline.
*   **on-hook:** The lifecycle signal that triggers the root of the pipeline (e.g., `job.completed`). Mutually exclusive with `on`.

---

## 2.3 Lifecycle Hooks

Lifecycle hooks allow for out-of-band orchestration triggered by the **Dispatcher** rather than a plugin event.

1.  **Opt-in:** A plugin must have `notify_on_complete: true` in its operator configuration.
2.  **Signal:** When the job reaches a terminal state, the Dispatcher resolves any pipelines matching the signal (e.g., `job.completed`).
3.  **Isolation:** Hook pipelines run as fresh root jobs with no context inheritance from the triggering job.

---

## 3. The Control Plane (Baggage & Ledger)

Every job in a pipeline is associated with an `event_context`.

### 3.1 `event_context` Schema
```sql
CREATE TABLE event_context (
  id               TEXT PRIMARY KEY,   -- UUID
  parent_id        TEXT,                -- FK for lineage
  pipeline_name    TEXT,
  step_id          TEXT,
  accumulated_json JSON NOT NULL,       -- The "Baggage"
  created_at       TEXT NOT NULL
);
```

### 3.2 Explicit Context Accumulation
Sprint 3 makes baggage explicit. Plugins emit event payloads; pipeline authors decide which values become durable.

When Step A transitions to Step B:
1.  Core reads `accumulated_json` from Step A's context.
2.  If Step B declares `baggage`, Core evaluates those claims against the immediate event `payload.*` and inherited `context.*`.
3.  Core deep-accretes the claimed values into a new `event_context` row for Step B.
4.  Existing durable paths are immutable. A step may add a new path or repeat the same value, but may not rewrite an inherited path.

Example:

```yaml
steps:
  - id: fetch
    uses: web_fetch
    baggage:
      web.url: payload.url

  - id: summarize
    uses: summarizer
    baggage:
      web.content: payload.content
      web.status_code: payload.status_code
```

Bulk import is allowed only under an explicit namespace:

```yaml
baggage:
  from: payload.metadata
  namespace: whisper
```

This imports `payload.metadata` as `context.whisper.*`. Omitting `namespace` is rejected until plugin manifest default namespaces exist.

During the Sprint 3 transition, a step with no `baggage` uses legacy shallow payload promotion. This preserves existing workflows while making migration visible through transition diagnostics.

---

## 4. The Data Plane (Workspace & Artifacts)

Every job is assigned a unique `workspace_dir` on the filesystem.

### 4.1 Lifecycle
1.  **Creation:** The Root job gets a fresh directory: `<workspace_root>/ws/<job_id>`.
2.  **Cloning (The Branch Mechanic):** When a pipeline `splits` or moves to the next step, the Core **clones** the workspace.
    *   To save space/time, the Core uses **Hard Links** (`cp -al`).
    *   This provides **Isolation**: Step B cannot accidentally delete a file needed by Step C in a parallel branch.
3.  **Retention (The Janitor):** Workspace directories are pruned after 24 hours (configurable).

---

## 5. The Plugin Protocol (v2)

Plugins receive the following via `stdin`:

```json
{
  "protocol": 2,
  "job_id": "uuid-456",
  "workspace_dir": "<workspace_root>/ws/job-456/",
  "context": {
    "origin_plugin": "discord",
    "channel_id": "123",
    "permission_tier": "WRITE"
  },
  "event": {
    "type": "video_downloaded",
    "payload": {
       "filename": "lecture.mp4",
       "size_bytes": 10485760
    }
  }
}
```

### 5.1 Plugin Responsibilities
*   **Metadata:** Read durable facts and routing info from `context`.
*   **Artifacts:** Read/Write files directly in `workspace_dir`.
*   **Communication:** Emit event payloads for downstream steps. Payload is per-hop; values become durable only when a pipeline author claims them with `baggage`.

---

## 6. Failure & Recovery

### 6.1 State Persistence
Because the `event_context` is in SQLite and artifacts are in the `workspace_dir`, a crash is non-destructive.
*   The **LLM Operator** can inspect the `event_context` to see exactly where a pipeline stalled.
*   The Core can "Replay" a step by creating a new job using the existing `event_context_id` and a cloned `workspace_dir`.

### 6.2 Cycle Detection
The Core maintains a `hop_count` in the `event_context`. If a pipeline exceeds 20 hops (or calls itself recursively too deep), the Core kills the chain to prevent infinite loops.

---

## 7. CLI & Operations

All orchestration-related CLI commands MUST support the following flags to ensure safety and observability:

- **-v, --verbose:** Expose internal DAG resolution, baggage merging logic, and path calculations.
- **--dry-run:** Preview the next steps of a pipeline without enqueuing jobs or cloning workspaces.

### 7.1 LLM Operator Affordances (RFC-004)

The Routing system exposes specific "Admin Utilities" for the LLM:
*   `job inspect <job_id>`: Returns the full Graph of what happened.
*   `pipeline visualize <name>`: Returns a Mermaid.js diagram of the DSL.
*   `pipeline dry-run <step_id>`: Clones the workspace to a `/sandbox/` directory and executes the plugin.

## 8. Branching & Decisions

Ductile supports two models for decision making: **Step-Gating (DSL)** and **Multi-Event Branching (Plugin)**.

### 8.1 Step-Gating via `if`

Pipelines can use the `if` keyword on any step to decide whether it should run based on the current payload, accumulated context, or plugin configuration.

```yaml
- id: notifier
  uses: discord-notifier
  if:
    path: payload.status
    op: eq
    value: error
```

Sprint 6 compiles authored `if:` conditions into an internal `core.switch` hop. That hop emits `ductile.switch.true` or `ductile.switch.false`, so the gated step only runs on the true branch while the false branch bypasses directly to the downstream route.

### 8.2 Multi-Event Branching

For complex domain-level decisions, plugins are responsible for emitting specific **Event Types** to signal different outcomes.

**Example Pipeline:**
```yaml
- id: validator
  uses: schema-checker
  # The router matches the emitted event type to the next pipeline or step.
```

This pattern keeps the DSL declarative while offloading complex logic to the plugins.
