# RFC-008: Project Identity Realignment — The Integration Engine for the Agentic Era

- **Status:** Proposed
- **Date:** 2026-02-28
- **Author:** @assistant (on behalf of User)
- **Tags:** [identity, architecture, nomenclature, marketing]

## 1. Executive Summary

Ductile is transitioning its primary identity from an "LLM Boundary Layer" to a **"Lightweight Open source Integration engine for the agentic era."** 

This realignment grounds the project in functional, real-world utility while maintaining its unique advantage as an LLM-operable system. We are establishing a robust compound semantic grounding that is comprehensible to human systems engineers, while providing high-value abstractions (LX/UX) for agentic operators.

## 2. Context & Problem Statement

Project direction has meandered as we explored the "Boundary Layer" and "Agentic" identities. While these are valuable, we must ensure they are built on a solid foundation of functional integration.

The current nomenclature has created semantic friction:
- Terms like "Skill" and "Affordance" have leaked into the core engine logic.
- We have used "LLM Boundary Layer" as the primary descriptor, which can be abstract and difficult for human operators to grasp.
- **"The skills/llm is an abstraction away from the core function. That is a UX/LS issue."**

We must return to the **"Integration sphere"** to ensure Ductile is perceived as a useful, quick to deploy, and extensible tool for building real-world automated workflows.

## 3. The New Identity

Ductile is:
- **Lightweight:** A single-binary Go runtime with SQLite. Easy to deploy anywhere.
- **Open Source:** A community-driven integration tool.
- **For the Agentic Era:** Built from the ground up to be discoverable, governable, and operable by LLM Agents (RFC-004, `--skills`).

## 4. The Ductile Integration Codex

To maintain clarity, we adopt the following unified nomenclature across the project:

| Term | Integration Semantic (Core/Human) | Agentic Era Context (LX/Agent) |
| :--- | :--- | :--- |
| **Gateway** | The runtime engine (Ductile). | The "Nervous System" of the host. |
| **Plugin** | A **Connector** bridging to an external system. | The source of new capabilities. |
| **Command** | A discrete **Operation** (poll, handle). | The specific **Skill** exposed to the LLM. |
| **Pipeline** | A defined, multi-hop **Orchestration**. | A governed, complex reasoning chain. |
| **Baggage** | **Stateful Metadata** (JSON) persisting across hops. | The LLM's "Working Memory" / tracing context. |
| **Workspace** | **Isolated Execution** environment. | The "Paper Trail" for auditing. |
| **Event** | A typed data packet triggering a transition. | - |
| **Job** | An immutable record of an operation's execution. | - |

## 5. Strategic Positioning

### Hobbyist & Enthusiast Pain Points
Ductile solves the "Integration Hell" for coders who want to build automated pipelines without the overhead of heavy SaaS tools or complex cloud infrastructure.
- **Problem:** "I want to wire my Discord to YouTube summaries and then push markdown to Astro, but I don't want to manage Zapier or a k8s cluster."
- **Solution:** Ductile. Quick to deploy, local-first, and extensible via simple Python/Go scripts.

### Marketing & Promotion
- **GitHub Description:** "Lightweight Open source Integration engine for the agentic era."
- **Primary Channels:** Hacker News (Show HN), Reddit (r/selfhosted, r/homelab), AI Dev Communities (Twitter/X).

## 6. Implementation Plan

### A. Core Glossary & Documentation
Create a **"Rock Solid Glossary"** and a **"Comprehensive Cookbook"** in the documentation index. The Cookbook will focus on common patterns like:
- Folder Watching -> AI Analysis -> File Writing.
- Discord Webhook -> Transcription -> Astro RSS.

### B. Code & TUI Synchronization
- Standardize the term **"Baggage"** in the TUI, logs, and CLI output.
- Update discovery endpoints (`GET /skills`) to identify themselves as the **"Connector Catalog"** in human documentation.
- Update CLI root help text in `cmd/ductile/main.go` to use the new identity and terminology.

### C. The RFC-004 Anchor
Frame `config` and `lock` commands as **"Operational Controls"** that allow the Gateway to be configured and secured by an LLM Operator.

## 7. Strategic Recommendations & Ownership

To ensure this identity realignment is not just a surface-level change, I recommend the following foundational shifts:

1.  **Protocol-Level "Baggage" Aliasing**: We should add a `baggage` field to the `protocol.Request` and `protocol.Response` structures. While `EventContext` works for the DB ledger, the plugins should see and speak "Baggage" explicitly.
2.  **Connector Manifest Enhancements**: We will update the plugin manifest to include a `capability` block. This block translates technical commands (e.g., `poll`, `handle`) into agent-friendly descriptions, effectively decoupling the **Operation** (functional) from the **Skill** (agentic).
3.  **The "Integration Sphere" Manual (`docs/CORE_MECHANICS.md`)**: A dedicated foundational document that explains Ductile purely in terms of Events, Jobs, and Workspaces. This is for the human architect who needs to trust the engine before handing it to an agent.
4.  **The "One-Minute Integration" Goal**: Focus on a highly optimized "Astro + Discord" Docker Compose example that stands up a functional integration in under 60 seconds, proving the "quick to deploy" claim.
5.  **Standardized Trace ID**: Deriving a `trace_id` from the root `event_context_id` that is present in every log line and baggage entry, making "Execution Lineage" a first-class citizen in the integration ledger.

## 8. Review & Critique (Action Items)

Reviewers are invited to critique this proposal and its initial implementation artifacts. Focus on whether we have achieved a **"robust compound semantic grounding"** in the following areas:

1.  **Identity & Positioning**: Critique `README.md` and `MANIFESTO.md` for their shift to the "Integration Engine" anchor.
2.  **The Codex**: Review `docs/GLOSSARY.md`. Does it effectively teach integration tech to human users while mapping agentic semantics?
3.  **The Interface**: Test `ductile --help` (in `cmd/ductile/main.go`). Does the language feel grounded in the "Integration Sphere"?
4.  **The Discovery Layer**: Inspect `GET /` and `GET /skills` (in `internal/api/handlers.go`). Does the discovery metadata reflect a "Connector Catalog" identity?

## 9. Expected Outcome

By grounding Ductile in the "Integration Sphere," we provide a stable foundation for human developers while fulfilling the promise of an "AI-Native" execution environment. We reduce cognitive load for humans and "inference friction" for agents, making Ductile the go-to tool for the next generation of automated systems.
