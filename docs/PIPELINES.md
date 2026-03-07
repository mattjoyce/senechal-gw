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

Path roots allowed in v1:
- `payload.*`
- `context.*`
- `config.*`

Semantics:
- typing is strict
- numeric operators require numeric operands
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

## 6. Validation

Ductile performs several checks when loading pipelines:
- **Cycle Detection:** Refuses to start if a pipeline calls itself (directly or indirectly).
- **Shadowing:** Ensures two pipelines don't use the same name.
- **Dangling Calls:** Ensures every `call` references a valid pipeline name.
- **Condition Validation:** Verifies `if` trees have valid shape, supported operators, allowed roots, and safe depth/count limits.
- **Schema Validation:** Verifies the YAML structure against the official [pipelines.json](schemas/pipelines.schema.json).
