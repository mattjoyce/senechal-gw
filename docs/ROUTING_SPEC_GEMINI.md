# Senechal Gateway — Routing & Orchestration Specification

**Version:** 1.0 (Gemini Consensus)  
**Date:** 2026-02-11  
**Model:** Governance Hybrid (DB + Workspace)

---

## 1. Overview

Senechal Gateway uses a **Graph-based Pipeline** model to orchestrate event flow. It separates **Governance** (metadata/context) from **Execution** (artifacts/workspace).

### 1.1 Core Components
*   **Control Plane (DB):** A SQLite ledger (`event_context`) that accumulates metadata ("Baggage") across hops.
*   **Data Plane (Filesystem):** A job-specific directory (`workspace_dir`) that holds large binary artifacts (audio, images, docs).
*   **Orchestrator (DSL):** A YAML-based Pipeline DSL that supports nesting, branching, and single-root triggers.

---

## 2. Pipeline DSL

Pipelines are defined in `pipelines.yaml` (or individual files in `pipelines/`).

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

### 3.2 Context Accumulation
When Step A transitions to Step B:
1.  Core reads `accumulated_json` from Step A's context.
2.  Core merges any new keys emitted by Step A's plugin.
3.  Core creates a **new** row in `event_context` for Step B.
4.  The "Origin Anchor" (initial trigger info) is preserved throughout the entire chain.

---

## 4. The Data Plane (Workspace & Artifacts)

Every job is assigned a unique `workspace_dir` on the filesystem.

### 4.1 Lifecycle
1.  **Creation:** The Root job gets a fresh directory: `/tmp/senechal/ws/<job_id>`.
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
  "workspace_dir": "/tmp/senechal/ws/job-456/",
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
*   **Metadata:** Read routing info from `context`.
*   **Artifacts:** Read/Write files directly in `workspace_dir`.
*   **Communication:** Only return filenames in the JSON `payload`, never the file content itself.

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

## 8. Branching & Decisions (Multi-Event Pattern)

To keep the DSL declarative and simple, Senechal avoids `if/else` logic in YAML. Instead, it uses **Multi-Event Branching**.

### 8.1 The Pattern
Plugins are responsible for decision-making. They inspect the data and emit a specific **Event Type** to signal the outcome.

**Example Pipeline:**
```yaml
- id: validator
  uses: schema-checker
  # No "if" keyword needed. The router matches the emitted event type.
  on_events:
    validation_success: [publisher]
    validation_failed: [error-notifier]
```

### 8.2 Benefits
*   **Decoupled Logic:** The Core doesn't need a complex expression evaluator.
*   **Explicit Ledger:** The decision point is recorded in the SQLite ledger as a specific event transition.
*   **Plugin Autonomy:** Plugins can use complex internal logic (AI, regex, DB lookups) to decide the path without bloating the DSL.
