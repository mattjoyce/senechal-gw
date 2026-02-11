# Senechal Gateway: Pipelines & Orchestration

This guide explains how to design, build, and debug event-driven workflows (Pipelines) in Senechal Gateway.

---

## 1. The Governance Hybrid Model

Senechal uses a unique "Governance Hybrid" architecture to manage data as it flows through a multi-hop chain. It separates **Governance** from **Execution**.

### 1.1 Control Plane (Baggage)
*   **What it is:** Metadata about the execution (e.g., `origin_user_id`, `discord_channel_id`, `trace_id`).
*   **How it works:** This data is stored in a SQLite ledger (`event_context`). It is automatically merged and carried forward (as "Baggage") to every job in the chain.
*   **Protection:** Keys starting with `origin_` are immutable. Once set at the start of a chain, plugins cannot overwrite them, ensuring a reliable audit trail.

### 1.2 Data Plane (Workspaces)
*   **What it is:** Large physical artifacts (e.g., `.mp3` audio files, `.txt` transcripts, `.pdf` documents).
*   **How it works:** Every job is assigned a `workspace_dir` on the filesystem.
*   **Isolation:** When a pipeline branches, Senechal performs a **Zero-Copy Clone** (using hardlinks). This ensures Step B and Step C have their own isolated folders but don't consume double the disk space.

---

## 2. Pipeline DSL

Pipelines are defined in YAML files located in `~/.config/senechal-gw/pipelines/`.

### 2.1 Basic Syntax

```yaml
pipelines:
  - name: video-wisdom
    on: discord.video_received   # The trigger event
    steps:
      - id: downloader
        uses: yt-dlp-plugin      # Execute a plugin
      
      - id: extractor
        uses: whisper-ai         # Next step in the chain
```

### 2.2 Reusable Middleware (`call`)
You can call one pipeline from another to promote logic reuse.

```yaml
  - name: standard-summarization
    on: audio.ready
    steps:
      - uses: whisper-ai
      - uses: llm-summarizer

  - name: discord-flow
    on: discord.link
    steps:
      - uses: downloader
      - call: standard-summarization  # Inherits baggage and workspace
```

### 2.3 Branching (`split`)
Use `split` to trigger multiple parallel paths.

```yaml
    steps:
      - uses: processor
      - split:
          - uses: discord-notifier
          - uses: s3-archiver
```

---

## 3. Decision Making: Multi-Event Branching

Senechal avoids `if/else` logic in YAML. Instead, the **Plugin** makes the decision by choosing which event type to emit.

### 3.1 The Pattern
1.  **Plugin:** Inspects the data and emits `quality_high` or `quality_low`.
2.  **YAML:** Routes those specific event types to different steps.

```yaml
- id: checker
  uses: quality-filter
  # The router implicitly matches the emitted event types
  on_events:
    quality_high: [publisher]
    quality_low: [reviewer]
```

---

## 4. Plugin Protocol (v2)

Plugins receive orchestration metadata via `stdin`.

```json
{
  "protocol": 2,
  "job_id": "uuid-456",
  "workspace_dir": "/tmp/senechal/ws/job-456/",
  "context": {
    "origin_user": "matt",
    "channel_id": "123"
  },
  "event": {
    "type": "video_downloaded",
    "payload": { "filename": "lecture.mp4" }
  }
}
```

**Plugin Checklist:**
*   Read `context` for routing baggage (like IDs).
*   Read/Write files directly in `workspace_dir`.
*   Only return filenames in the JSON `payload`, never file content.

---

## 5. Troubleshooting & Observability

### 5.1 The `inspect` Tool
Use the `inspect` command to visualize the "Lineage" of a job. It shows exactly how the baggage accumulated and which files exist in each workspace.

**CLI Principles:**
All Senechal CLI commands support:
*   `-v, --verbose`: To see internal routing decisions and baggage merges.
*   `--dry-run`: To preview pipeline transitions without executing code.

```bash
senechal-gw inspect <job_id> -v
```

**Output Example:**
```text
[1] <root> :: <entry>
    context_id : uuid-ctx-1
    baggage    : {"origin_user": "matt"}
    artifacts  : [video.mp4]

[2] video-wisdom :: step_process
    context_id : uuid-ctx-2
    parent_id  : uuid-ctx-1
    baggage    : {"origin_user": "matt", "status": "processed"}
    artifacts  : [video.mp4, summary.txt]
```

### 5.2 Cycle Detection
Senechal automatically detects circular dependencies (e.g., A calls B, B calls A) at load time and will refuse to start if a cycle is found.
