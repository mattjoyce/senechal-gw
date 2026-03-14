# RFC-005: First-Class Orchestration Primitives (The "Structural DSL" Update)

**Status:** Draft / Proposal  
**Author:** Gemini CLI (on behalf of Ductile)  
**Date:** 2026-03-05  

---

## 1. THE WHY: The Case for Native Logic

### 1.1 From "Connector Gateway" to "Programmable Control Plane"
Ductile currently treats all logic as a Plugin concern (**Idiom #2**). While this keeps the Go core simple, it forces structural flow—like branching, looping, and error handling—into a "black box" process spawn. 

The **Switch Plugin** is a primary example of a "leaky abstraction." Nobody's goal is to "use the Switch action"; they simply need to make a choice. By forcing this into a plugin, we treat "making a decision" as a heavy-duty job requiring a subprocess, a JSON handshake, and a database hit.

### 1.2 Efficiency (The Latency Gap)
A decision in Ductile via a plugin adds **15–50ms** per hop. In a high-volume environment or a deep pipeline, this overhead compounds. Moving this to the Go core reduces decision latency to **<1ms**.

### 1.3 Observability (The "Overwatch" Win)
When logic is hidden inside a plugin's `config:`, it is invisible to the system's "eye." By promoting orchestration to the DSL:
- **Decision Diamonds:** The TUI can render clear visual branches in the execution DAG.
- **Iteration Stacks:** The system can visualize a "loop" in progress, showing the state of multiple parallel child jobs.

---

## 2. THE WHAT: The 4 Structural Primitives

We propose introducing four (4) first-class DSL keywords to the `StepSpec` schema. These primitives are evaluated directly by the Go core's dispatcher.

### 2.1 `if` (Conditional Step Execution)
Allows a step to be skipped based on a boolean evaluation of the current **Payload** or **Baggage**.
- **GHA Equivalent:** `if: github.event_name == 'push'`
- **Ductile Benefit:** "Send a Discord alert **only if** the previous step failed or returned a specific status."
- **Example:**
  ```yaml
  - uses: discord-notify
    if: "payload.status == 'error'"
  ```

### 2.2 `foreach` (Dynamic Fan-out / Matrix)
Enables iteration over a JSON array in the payload. The Core handles the loop, enqueuing a separate job for each item.
- **GHA Equivalent:** `strategy: matrix: { version: [10, 12, 14] }`
- **Ductile Benefit:** "For every video in my `new_playlist` list, run the `transcript` plugin in parallel."
- **Example:**
  ```yaml
  - foreach: "payload.items"
    uses: processor
  ```

### 2.3 `continue_on_error` (Soft Failure)
Prevents a step's failure from terminating the entire pipeline.
- **GHA Equivalent:** `continue-on-error: true`
- **Ductile Benefit:** "Try to ping my status page. If it fails (maybe the internet is flaky), just keep going with the rest of the pipeline."
- **Example:**
  ```yaml
  - uses: health-ping
    continue_on_error: true
  ```

### 2.4 `log` (Native Instrumentation)
Allows the pipeline to emit structured logs directly into the **Execution Ledger** without spawning a plugin.
- **Ductile Benefit:** High-speed tracing of pipeline state without the overhead of a logging plugin.
- **Example:**
  ```yaml
  - log: "Processing {payload.video_id} for user {context.origin_user}"
  ```

---

## 3. THE HOW: Technical Implementation

### 3.1 The "Dispatcher Interceptor"
The Go Dispatcher (`internal/dispatch/`) will be updated with an **Orchestration Interceptor**. Before spawning a plugin process:
1. **Intercept `log`:** Interpolate variables, write to SQLite, and proceed immediately.
2. **Intercept `if`:** Evaluate the expression. If `false`, record the node as `skipped` and move to the next node.
3. **Intercept `foreach`:** 
   - Pause the parent job.
   - For each item in the array: Create a new `job_queue` entry.
   - Perform a **Zero-Copy Clone** (`cp -al`) of the parent's `workspace_dir` to the child's dir.
   - Resume when all children reach a terminal state (for `synchronous` pipelines).

### 3.2 The Minimalist Expression Engine
We will avoid full CEL (Common Expression Language) syntax to keep the core lightweight. We will use a **Strictly Limited Evaluator** (e.g., `antonmedv/expr` or a custom regex-based matcher) that is:
- **Sandboxed:** No access to system calls or filesystem.
- **Time-Bounded:** Evaluation is killed if it takes more than 10ms.
- **Read-Only:** It can only inspect `payload`, `context`, and `config`.

---

## 4. RISKS & BLINDSPOTS (Safety Rails)

| Keyword | Primary Risk | Blindspot / Unintended Consequence | Mitigation |
| :--- | :--- | :--- | :--- |
| **`if`** | **YAML Scripting:** Brittle, hard-to-test logic in YAML. | **Security Surface:** Expression engine could be a vector for escape. | **Strict Syntax:** No loops or recursion. Evaluation capped at <10ms. |
| **`foreach`** | **Resource Exhaustion:** A "Fork Bomb" of jobs. | **Inode Exhaustion:** Each clone consumes disk inodes. | **`max_fanout` Limit:** Default cap (e.g., 50 items) per loop. |
| **`continue`** | **Silent Corruption:** Missing data for next step. | **Success Masking:** Pipeline reports success while failing every action. | **Visual Alert:** TUI renders "Continued with Errors" in Amber/Warning color. |
| **`log`** | **Log Flooding:** Disk saturation in large loops. | **Secret Leak:** Accidentally logging `payload.token`. | **Rate Limiting:** Max 100 entries per run. **Redaction Filter** for sensitive keys. |

---

## 5. SUMMARY: The Architectural Shift

By adopting these primitives, we are formalizing **Idiom #11**:
> **"The Core owns the Flow; the Plugin owns the Skill."**

Ductile remains a lightweight, polyglot engine, but its **Orchestration Map (YAML)** becomes a powerful, observable control plane, while the **Connectors (Plugins)** remain focused on their specific domain expertise.
