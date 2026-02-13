# Sprint 4: Orchestration & The Governance Hybrid

**Status:** Strategic Blueprint  
**Goal:** Implement the "Governance Hybrid" (DB + Workspace) and the Pipeline Router.  
**Focus:** Order of operations, coordination safety, and future-proofing against "Developer Regret."

---

## 1. Coordination Strategy (Multiple Agents)

To ensure multiple agents (or humans) can work in parallel without causing a "code tangle," we will enforce **Strict Module Isolation** via Interface-First design.

1.  **The Contract Layer:** Before any logic is written, we define the Interfaces in `internal/router/interfaces.go` and `internal/workspace/interfaces.go`.
2.  **The Mock Layer:** We generate mocks for these interfaces. Agent A can build the `Router` using a `MockWorkspaceManager`, while Agent B builds the actual `Workspace` implementation.
3.  **The Integration Layer:** Only once individual tests pass do we wire the concrete implementations together in `cmd/ductile/main.go`.

---

## 2. Order of Operations (Phase-by-Phase)

### Phase 1: The Data Plane (Workspaces)
*   **Objective:** Give every job a physical "home" for artifacts.
*   **Step 1.1:** Define `workspace.Manager` interface (Create, Clone, Open, Cleanup).
*   **Step 1.2:** Implement `fsWorkspaceManager`. Use **Hardlinks** (`os.Link`) for cloning. This is the "Zero-Copy" branch mechanic.
*   **Step 1.3:** Implement the **Janitor**. A background loop that prunes workspaces based on `job_log` timestamps.
*   **Step 1.4:** Update `internal/plugin` (Protocol v2) to inject the `workspace_dir` path into the plugin's stdin.

### Phase 2: The Control Plane (Baggage & Lineage)
*   **Objective:** Build the searchable "Execution Ledger."
*   **Step 2.1:** Database Migration. Add `event_context` table (id, parent_id, accumulated_json).
*   **Step 2.2:** Update `job_queue` to include `event_context_id`.
*   **Step 2.3:** Implement **Baggage Middleware**. This logic sits in the `Dispatcher`. It reads the plugin response, merges the `state_updates` with the parent `event_context`, and saves the result as a *new* context row.

### Phase 3: The DSL & Compiler
*   **Objective:** Turn YAML into a validated Directed Acyclic Graph (DAG).
*   **Step 3.1:** Define the Pipeline YAML schema (on, steps, call, split).
*   **Step 3.2:** Build the `dsl.Compiler`. It must:
    *   Detect circular dependencies.
    *   Verify all `uses` plugins exist.
    *   Generate a **BLAKE3 Hash** for the pipeline version.
*   **Step 3.3:** Multi-file discovery. Load all pipelines from `~/.config/ductile/pipelines/`.

### Phase 4: The Router Hook (The "Big Bang")
*   **Objective:** Connect the dots.
*   **Step 4.1:** Implement the `Router.Next(event, current_context)` method. It looks up the DAG and returns the next jobs to enqueue.
*   **Step 4.2:** Hook the Router into the `Dispatcher`. When a job succeeds, the Dispatcher calls the Router and enqueues the resulting "Next Steps."
*   **Step 4.3:** Traceability. Ensure `parent_job_id` and `event_context_id` are correctly propagated.

---

## 3. Code Quality & "Anti-Regret" Standards

### 3.1 Workspace Safety
*   **Hardlinks Only:** Never perform a "Deep Copy" of a workspace unless explicitly requested. Hardlinks are fast and save disk space.
*   **Atomic Completion:** A plugin's artifacts are only considered "committed" if the job status in the DB is `succeeded`.

### 3.2 Baggage Integrity
*   **The Origin Anchor:** Metadata starting with `origin_` (e.g., `origin_user_id`) is **read-only** for plugins. The Core will reject any attempt by a plugin to overwrite these keys.
*   **Namespace Encouragement:** Core logs a `WARN` if a plugin emits top-level keys that conflict with standard system keys.

### 3.3 Path Portability
*   **Absolute Path Ban:** The `event_context` and `job_queue` tables MUST NOT store absolute paths. They store `job_id`. The Core calculates the path at runtime (e.g., `/base/dir/ + job_id`). This allows you to move your entire Ductile data directory to a new drive without breaking the database.

---

## 4. Success Criteria: The "Wisdom Loop" Verification

The sprint is done when we can run the following test:

1.  **Trigger:** `POST /webhooks/discord` (containing a YouTube URL).
2.  **Ledger:** DB shows a chain of 5 distinct jobs.
3.  **Context:** The final `discord-notifier` job receives a JSON payload that contains the `channel_id` from the *first* job, even though the middle 3 plugins didn't know it existed.
4.  **Workspace:** The final job can read `summary.txt` from its workspace, which was created 2 steps prior.
5.  **Artifacts:** The `/tmp/ductile/ws/` folder for this chain is automatically deleted 24 hours later.
