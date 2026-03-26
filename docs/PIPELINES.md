# Ductile: Pipelines & Orchestration (DSL Reference)

Ductile uses a YAML-based Domain Specific Language (DSL) to define event-driven workflows. Pipelines transform atomic **Connectors** into complex, multi-hop **Orchestrations**.

---

## 1. Top-Level Structure

A pipeline file (e.g., `pipelines.yaml`) contains an array of pipeline definitions.

```yaml
pipelines:
  - name: my-workflow      # Required: Unique identifier
    on: my.event.type      # Required: Trigger event type
    execution_mode: async  # Optional: async (default) | synchronous
    timeout: 30s           # Optional: For synchronous execution
    steps:                 # Required: Sequential steps
      - uses: my-plugin
```

---

## 2. Pipeline Properties

| Field | Type | Description |
|-------|------|-------------|
| `name` | String | A unique name for the pipeline. Used for logging and API triggers. |
| `on` | String | The event type that triggers this pipeline. Must match exactly. |
| `execution_mode`| Enum | `async` (fire-and-forget) or `synchronous` (API blocks for result). |
| `timeout` | Duration| Max time to wait for a `synchronous` pipeline (e.g., `5s`, `2m`). |
| `steps` | Array | The list of steps to execute in order. |

---

## 3. Step Types

Each step in a pipeline must perform exactly **one** of the following actions:

### 3.1 `uses` (Invoke Plugin)
Calls a specific plugin or alias. This is the most common step.

```yaml
steps:
  - id: download-step   # Optional: Unique ID within the pipeline
    uses: youtube-dl
```

### 3.2 `call` (Invoke Pipeline)
Calls another pipeline by name, inheriting the current baggage and workspace. This promotes logic reuse.

```yaml
steps:
  - call: standard-summarizer
```

### 3.3 `steps` (Nested Sequence)
Groups multiple steps together. Useful for organization or within a `split`.

```yaml
steps:
  - steps:
      - uses: step-1
      - uses: step-2
```

### 3.4 `split` (Parallel Fan-out)
Executes multiple steps or sub-pipelines in parallel. Ductile ensures each branch has its own isolated **Workspace** (via hard-links) while sharing the same **Baggage**.

```yaml
steps:
  - uses: processor
  - split:
      - uses: discord-notifier
      - uses: s3-archiver
```

### 3.5 `if` (Conditional Step Execution)
A step may include an optional structured `if` object. If the condition evaluates to `false`, Ductile marks the step as `skipped`, records the reason, and continues to downstream steps without spawning the plugin process.

`if` must be exactly one of:
- atomic predicate: `path`, `op`, optional `value`
- `all: [...]`
- `any: [...]`
- `not: <predicate>`

Atomic example:

```yaml
steps:
  - uses: discord-notify
    if:
      path: payload.status
      op: eq
      value: error
```

Composite example:

```yaml
steps:
  - uses: long-video-handler
    if:
      all:
        - path: payload.kind
          op: eq
          value: video
        - path: payload.duration_sec
          op: gte
          value: 30
```

Supported operators in v1:
- `exists`
- `eq`
- `neq`
- `in`
- `gt`
- `gte`
- `lt`
- `lte`
- `contains` (case-insensitive string contains)
- `startswith` (case-insensitive string prefix)
- `endswith` (case-insensitive string suffix)
- `regex` (Go regexp full-string match; use inline flags like `(?i)` for case-insensitive patterns)

Path roots allowed in v1:
- `payload.*`
- `context.*`
- `config.*`

Semantics:
- typing is strict
- numeric operators require numeric operands
- string operators require string path values and string comparison values
- no implicit string-to-number coercion
- missing paths resolve to absent for `exists`, otherwise compare as `null`
- invalid conditions fail at pipeline load time

---

## 4. How Data Flows

### 4.1 The Data Plane (Workspaces)
- Every job gets a unique folder on disk.
- If Step A creates `video.mp4`, Step B can read it from its own workspace.
- When a `split` occurs, both branches get a copy of the parent's files (zero-copy clone).

### 4.2 The Control Plane (Baggage)
- Metadata (JSON) is stored in the `event_context` database table.
- Every step automatically receives all metadata produced by upstream steps.
- Immutable keys (starting with `origin_`) are preserved for the entire life of the pipeline.

### 4.3 Results & Payloads
- The `result` (short string) and `payload` (JSON) from Step A are passed to Step B.
- In `synchronous` mode, the final API response aggregates the results from every step.
- **Synthetic events:** If a pipeline step completes successfully but emits no events, Ductile routes a synthetic `ductile.step.succeeded` event to ensure downstream sequential steps are still triggered.

---

## 5. Decision Making

Ductile supports two kinds of decision making:

### 5.1 Native step gating with `if`
Use `if` when you want to decide whether a step should run based on the current payload, accumulated context, or plugin config.

### 5.2 Event-driven branching
Ductile also supports **Event-Driven Branching**. A plugin decides the next path by choosing which event type to emit.

1.  **Step 1:** Plugin `classifier` inspects data.
2.  **Output:** Plugin emits `type: "image.detected"` or `type: "text.detected"`.
3.  **Routing:** You define two pipelines—one `on: image.detected` and one `on: text.detected`.

Use this when the plugin is making a domain decision about what happened. Use `if` when the pipeline is making a structural decision about whether a step should run.

---

## 6. Dispatcher Preflight

Before spawning a plugin process, the dispatcher runs a **preflight phase** for every job. Preflight separates orchestration decisions from plugin execution, ensuring consistent data-plane semantics regardless of whether a step runs or is skipped.

### 6.1 Preflight Steps

Preflight executes three operations in order:

1. **Load request context** — Fetches accumulated baggage from the `event_context` table (all upstream metadata for this job's execution tree).

2. **Ensure workspace** — Creates or inherits a workspace directory for the job. If a parent job exists, the workspace is cloned from it (zero-copy hard-link clone). If no parent exists, a fresh workspace is created. This runs *before* condition evaluation so that skipped steps still have a valid workspace for downstream inheritance.

3. **Evaluate `if` condition** — If the job's pipeline step has an `if` condition, evaluates it against the available scope (`payload.*`, `context.*`, `config.*`). Only runs when the job is part of a pipeline with a router.

### 6.2 Preflight Outcomes

| Outcome | When | Effect |
|---------|------|--------|
| **run** | No `if` condition, or condition evaluates `true` | Plugin process spawns normally |
| **skip** | `if` condition evaluates `false` | No plugin spawns; job marked `skipped`; synthetic `ductile.step.skipped` event routed to downstream steps |
| **fail** | Context load, workspace creation, or condition evaluation returns an error | Job marked `failed`; no plugin spawns; no downstream routing |

### 6.3 Workspace Inheritance for Skipped Steps

A skipped step's workspace is created during preflight, before the condition is evaluated. This means:

- Downstream steps that inherit from a skipped parent receive a valid workspace (cloned from the skipped step's parent).
- Workspace clone/inheritance is independent of whether the parent ran or was skipped.
- No data-plane inconsistency: every job in the execution tree has a workspace, regardless of its terminal status.

### 6.4 Skipped Step Routing

When a step is skipped, the dispatcher:

1. Publishes a `job.skipped` event (with `reason`) to the event hub.
2. Routes a synthetic `ductile.step.skipped` event through the router, allowing downstream steps to execute.
3. Marks the job as `skipped` with a synthetic result payload: `{"status": "skipped", "reason": "..."}`.

Successor routing happens *before* the job is marked terminal, preventing synchronous callers from seeing the tree as complete before all children are enqueued.

### 6.5 Preflight Events

The dispatcher emits a `job.preflight` event after preflight completes (or fails), with the following payload:

```json
{
  "job_id": "uuid",
  "plugin": "plugin-name",
  "command": "command-name",
  "decision": "run | skip | fail",
  "reason": "",
  "workspace_dir": "/path/to/workspace"
}
```

The `reason` field is empty for `run` decisions, contains the condition failure reason for `skip`, and contains the error message for `fail`. These events enable async consumers (TUI, event streams, monitoring) to distinguish orchestration decisions from plugin execution outcomes.

---

## 8. Lifecycle Hooks (`on-hook`)

Lifecycle hooks allow pipelines to trigger based on **system events** (e.g., job completion) rather than plugin-emitted events. Hook pipelines run as independent root jobs and do not inherit context from the job that triggered them.

### 8.1 DSL Syntax

Use the `on-hook:` keyword instead of `on:`. These keywords are mutually exclusive.

```yaml
pipelines:
  - name: notify-on-failure
    on-hook: job.completed
    steps:
      - uses: discord-notify
        if:
          path: payload.status
          op: neq
          value: succeeded
```

### 8.2 Supported Signals

| Signal | Triggered When |
|--------|----------------|
| `job.completed` | A root job reaches a terminal state (`succeeded`, `failed`, `timed_out`, or `dead`). |

### 8.3 Opt-in Configuration

To prevent accidental infinite loops and reduce noise, plugins must explicitly opt-in to lifecycle hooks in their configuration.

```yaml
plugins:
  my-important-plugin:
    notify_on_complete: true  # Required for on-hook: job.completed to fire
```

---

## 9. Failure States & Event Payloads

When a job fails, times out, or becomes "dead" (exceeds retries), Ductile emits specialized events. These events include enhanced payloads to simplify downstream notifications.

### 9.1 Enhanced Payload Fields

In addition to standard fields like `job_id` and `duration_ms`, failure events (`job.failed`, `job.timed_out`, `job.dead`) include:

| Field | Description | Example |
|-------|-------------|---------|
| `plugin` | The name of the plugin that failed. | `git-sync` |
| `message` | A human-readable summary of the failure. | `Job failed [git-sync]: connection reset` |
| `text` | An alias for `message` (convenience for notification plugins). | `Job failed [git-sync]: connection reset` |
| `error` | The raw error message (if available). | `connection reset` |

### 9.2 Usage in Pipelines

These fields enable simple notification steps without complex `if` logic or payload mapping:

```yaml
pipelines:
  - name: failure-announcer
    on-hook: job.completed
    steps:
      - uses: discord-notify
        if:
          path: payload.status
          op: neq
          value: succeeded
        # discord-notify automatically uses payload.message if present
```

---

## 10. Validation


Ductile performs several checks when loading pipelines:
- **Cycle Detection:** Refuses to start if a pipeline calls itself (directly or indirectly).
- **Shadowing:** Ensures two pipelines don't use the same name.
- **Dangling Calls:** Ensures every `call` references a valid pipeline name.
- **Condition Validation:** Verifies `if` trees have valid shape, supported operators, allowed roots, and safe depth/count limits.
- **Schema Validation:** Verifies the YAML structure against the official [pipelines.json](schemas/pipelines.schema.json).
