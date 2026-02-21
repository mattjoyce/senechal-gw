# RFC-005-Critique-Gemini: Routing Payload & Context Management

**Status:** Historical Critique  
**Date:** 2026-02-11  
**Author:** Gemini (Interactive CLI Agent)  
**Context:** Critique of RFC-005 Routing Payload Management Strategies

---

## 1. The Core Tension: Simple Plugins vs. Robust Orchestration

The fundamental challenge in RFC-005 is deciding who pays the "complexity tax" for multi-hop state. 

*   **Option 1 (Baggage Forwarding)** taxes the **plugin author**. It requires every developer to write boilerplate code to preserve state they don't understand or care about.
*   **Option 2 (DB-Mediated)** taxes the **gateway core**. It makes the core "smarter" by handling merge logic and state persistence.

**My Position:** For a personal integration gateway, **Option 2 (Database-Mediated Context)** is the correct architectural choice, provided we solve the "Payload Explosion" problem. The gateway's primary value proposition is *orchestration*; if plugins have to manage their own orchestration context, the gateway is just a subprocess runner.

---

## 2. Evaluation of Options

### Option 1: Baggage Forwarding (Explicit Pass-Through)
*   **Verdict:** ❌ **Too brittle.** 
*   **Reasoning:** In a personal project, "discipline" is the first thing to fail. If I write a quick `bash` script plugin to hit a URL, I am 90% likely to forget to pass through `channel_id`. Debugging why a notification didn't fire 4 hops later because of a missing key in a middle-man script is a miserable user experience.

### Option 2: Database-Mediated Context (Accumulated Payloads)
*   **Verdict:** ✅ **Strongest Foundation.**
*   **Reasoning:** SQLite is remarkably efficient at this scale. Storing 1-5KB of JSON per event hop is trivial for a 50-job-per-day system. It enables "Time Travel Debugging"—the ability to look at any job in the history and see exactly what the *entire* state of the world was at that moment.

### Option 3: Hybrid (Context References + Inline Data)
*   **Verdict:** ⚠️ **Over-engineered for now.**
*   **Reasoning:** This introduces two distinct communication channels. While logically sound, it increases the cognitive load for plugin authors ("Is this a context key or a payload key?"). 

### Option 5: The Workspace Hybrid (JSON Metadata + Filesystem Blobs)
*   **Verdict:** 🏆 **The Winner (Consensus).**
*   **Reasoning:** This solves the "Binary Problem" (audio/video) while keeping the "Baggage Problem" (orchestration) simple. The Core manages a small JSON context (Baggage) in the DB, while large data is written to a job-specific directory on disk.
*   **Implementation:** Plugins receive a `workspace_dir` path in their JSON request.

---

## 3. The "Blind Spots" (What RFC-005 Missed)

### 3.1 The "Context Poisoning" Problem
If we use **Option 2 (Accumulated)**, what happens if `Plugin B` and `Plugin C` both try to set a key named `status`? 
*   **RFC-005 says:** "Last-write-wins."
*   **The Risk:** A downstream plugin might overwrite critical metadata from an upstream plugin.
*   **Better Approach:** Namespace the accumulated context? `context["withings"]["last_sync"]` instead of just `context["last_sync"]`.

### 3.2 Security & Data Leaking
In a multi-hop chain, `Plugin E` (a simple logger) might receive sensitive tokens or PII (Personal Identifiable Information) accumulated by `Plugin A` (an authenticator). 
*   **The Blind Spot:** We need a way to mark keys as "Transient" (payload only, don't persist) or "Persistent" (context).

### 3.3 The Fan-Out "Split Personality"
If Event A triggers Job B and Job C:
*   Do B and C share the same `event_context_id`?
*   If Job B completes first and updates the context, does Job C see those updates?
*   **Recommendation:** Context should be **immutable per hop**. Every job creates a *new* context record that branches from its parent. This prevents race conditions even if we eventually move to parallel dispatch.

### 3.4 Chain Depth & Cycle Detection
With accumulated context, an accidental routing loop (A → B → A) becomes a "memory leak" in the database as the `accumulated` column grows with every hop.
*   **The Blind Spot:** RFC-005 doesn't specify a "TTL" or "Max Hops" for an event chain.
*   **Recommendation:** The Core should inject a `hop_count` into the context and kill any chain that exceeds a reasonable limit (e.g., 20 hops).

---

## 4. The Gemini Recommendation: "The Governance Hybrid" (Control vs. Data Plane)

In light of **RFC-004 (LLM-as-Operator)**, I recommend a design that separates **Governance** from **Execution**.

### 4.1 The Architecture
1.  **Control Plane (SQLite):** Manages the **Execution Ledger**. It accumulates "Baggage" (metadata like `channel_id`, `trace_id`, and `permission_tier`) across hops. This allows the LLM Operator to inspect the "State of the World" at any point in a chain.
2.  **Data Plane (Filesystem):** Manages **Artifacts**. Every job gets a `workspace_dir`. Large binary data (audio, images) and transient files (transcripts) live here, isolated from the database and the Core's memory.

### 4.2 Alignment with RFC-004
*   **Ledger Inspection:** The LLM can query the DB to see the full context of a multi-hop chain without manual stitching.
*   **Artifact Management:** The `workspace_dir` provides a structured location for the "Artifacts" defined in RFC-004 Section 7.
*   **Safety Envelope:** The Filesystem allows for easy "Sandbox Dry-runs" by cloning workspace directories before executing "DANGEROUS" tier skills.

### 4.3 Implementation Summary
*   **Core:** Injects `context` (JSON metadata) and `workspace_dir` (Path) into every plugin.
*   **Plugins:** Read metadata from `stdin`, read/write artifacts from `workspace_dir`.
*   **Retention:** The "Janitor" prunes the Data Plane (Filesystem) frequently, while the Control Plane (DB Ledger) retains metadata for the standard audit window (e.g., 30 days).

---

## 5. Final Verdict for MVP

**Do not go with Option 1.** It feels simple now but will lead to a "silent failure" debugging nightmare. 

**Go with Option 2 (Database-Mediated), but keep it simple:**
*   Add the `event_context` table.
*   The `accumulated` column is the source of truth for downstream `handle` commands.
*   If a key exists in the parent context and the new event payload, the payload value wins (standard merge).
*   **Namespace by default:** Encourage plugins to nest their data under their own name to avoid collisions.
