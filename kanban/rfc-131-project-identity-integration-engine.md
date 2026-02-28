---
id: 131
status: doing
priority: High
blocked_by: []
tags: [rfc, architecture, nomenclature, documentation, marketing]
---

# RFC-131: Realign Project Identity as an Integration Engine

## Goal
Re-cast Ductile's core identity as a **"Lightweight Open source Integration engine for the agentic era"**. While the system provides robust LLM affordances (LX, RFC-004), its foundation must be grounded in the functional reality of an integration system, utilizing **"robust compound semantic grounding"** that is comprehensible to human operators. 

## Situation
The project direction has meandered slightly, leading to mixed abstractions. Terms like "Skill," "Affordance," and "LLM Boundary Layer" have leaked down into core engine concepts, causing semantic friction. As noted: **"The skills/llm is an abstraction away from the core function. That is a UX/LS issue."** We need to separate the underlying **"Integration sphere"** (the reality of the engine) from the Agentic Readiness layer (how LLMs interact with it).

## Proposed Action: Root and Branch Review
Conduct a root and branch review of the codebase, configuration, and documentation to enforce a unified **Ductile Integration Codex**. 

### The Ductile Integration Codex (Proposed)

| Term | Integration Semantic (Core) | Agentic Era Context (LX) |
| :--- | :--- | :--- |
| **Gateway** | The runtime engine (Ductile). | The "Nervous System" of the host. |
| **Plugin** | A **Connector** bridging to an external system. | The source of new capabilities. |
| **Command** | A discrete **Operation** (poll, handle). | The specific **Skill** exposed to the LLM. |
| **Pipeline** | A defined, multi-hop **Orchestration**. | A governed reasoning chain. |
| **Baggage** | **Stateful Metadata** persisting across hops. | The LLM's "Working Memory" / tracing context. |
| **Workspace** | **Isolated Execution** environment. | The "Paper Trail" for auditing. |
| **Event** | Typed packet triggering transitions. | - |
| **Job**| The immutable record of execution. | - |

## Marketing & Positioning Decisions

### GitHub Description
**"Lightweight Open source Integration engine for the agentic era."** (Direct and punchy).

### README Positioning
The README must lead with the **"functional goals of being a useful, quick to deploy, extensible integration engine."** It should frame "Agentic Readiness" as a superpower of a rock-solid integration tool, not the tool's only reason for existing.

### Hobbyist/Enthusiast Pain Points
Ductile solves the "Integration Hell" for coders who want to wire things up (e.g., Discord -> YouTube -> Astro) without heavy infrastructure.
- **Pain Point:** "I want to automate X but don't want to manage a complex k8s cluster or pay for expensive SaaS automation."
- **Ductile Solution:** "A single-binary Go gateway that is easy to deploy, polyglot (write in Python/Node/Go), and AI-ready by default."

### Promotion Channels
- **Show HN (Hacker News):** Focus on the "Lightweight" and "Polyglot" aspects.
- **Self-Hosted / HomeLab Communities:** Focus on the single-binary and local-first execution.
- **AI Dev Twitter/X:** Focus on the "LX is the New UX" and RFC-004 (LLM-operable config).

## Assistant Recommendations & Ownership

As we realign Ductile toward its core as a **Managed Integration Gateway**, I recommend the following "deep-tissue" changes to ensure the semantic grounding is unbreakable:

1.  **Protocol Level Aliasing**: We should add a `baggage` field to the `protocol.Request` and `protocol.Response` structures. While `EventContext` works for the DB ledger, the plugins should see and speak "Baggage".
2.  **Connector Manifests**: Enhance the plugin manifest to include a `capability` block. This block should translate technical commands (`poll`, `handle`) into agent-friendly descriptions, effectively decoupling the **Operation** (functional) from the **Skill** (agentic).
3.  **The "Integration Sphere" Manual**: Create a new foundational doc, `docs/CORE_MECHANICS.md`, which explains Ductile purely in terms of Events, Jobs, and Workspaces. This is for the human architect who needs to trust the engine before handing it to an agent.
4.  **Hobbyist "One-Liner" deployment**: We need a `curl | bash` or a highly optimized Docker Compose that stands up a useful "Astro + Discord" integration in under 60 seconds. This hits the "quick to deploy" goal.
5.  **Unified Trace ID**: Standardize on a `trace_id` (derived from the root `event_context_id`) that is present in every log line and baggage entry, making "Execution Lineage" a first-class citizen in the integration ledger.

## Files for Review (Reviewer Guidance)

Reviewers are requested to critique the following artifacts to ensure they align with the goal of a **"Managed Integration Gateway"** grounded in the **"Integration Sphere"**:

1.  **Core Proposal**: `RFC/RFC-008-Integration-Engine-Identity.md`
2.  **The New Front Door**: `README.md`
3.  **The Vision**: `MANIFESTO.md`
4.  **The Integration Codex**: `docs/GLOSSARY.md` (Crucial for semantic grounding).
5.  **CLI Experience**: `cmd/ductile/main.go` (See `printUsage` function).
6.  **API Discovery**: `internal/api/handlers.go` (See `handleRoot` and `handleListSkills`).

## Acceptance Criteria
- [ ] **RFC-008**: Draft formal RFC document detailing the identity shift.
- [ ] **Glossary**: Create a "Rock Solid Glossary" in the docs index.
- [ ] **Cookbook**: Build a comprehensive cookbook for common use case patterns (Discord, YouTube, Astro, etc.).
- [ ] **README**: Rewrite `README.md` using the "Integration Engine" anchor.
- [ ] **CLI Help**: Update root help in `cmd/ductile/main.go` to use consistent integration terminology.
- [ ] **API Metadata**: Reframe discovery endpoints (`GET /skills`) in human docs as "Connector Catalogs".

## Narrative
- 2026-02-28: Created card to firmly ground the project identity in the "Integration Sphere". Formulated the "Ductile Integration Codex" to resolve semantic friction. (by @assistant)
- 2026-02-28: Updated card to include GitHub/README positioning, hobbyist pain points, and promotion strategy. Embedded the user's specific phrases ("Lightweight Open source Integration engine for the agentic era", "Integration sphere", "robust compound semantic grounding", "Skill/llm is an abstraction") to ensure reviewer alignment. (by @assistant)
- 2026-02-28: Completed "Root and Branch" realignment of README, Manifesto, CLI Help, and API Discovery. Drafted RFC-008 and added owner recommendations for protocol-level "Baggage" and capability-agnostic manifests. Ready for review. (by @assistant)
