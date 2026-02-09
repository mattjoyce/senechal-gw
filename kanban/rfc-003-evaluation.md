---
id: 27
status: backlog
priority: Normal
blocked_by: [21, 22, 23]
tags: [planning, architecture, future]
---

# RFC-003 Agentic Loop Runtime - Evaluation & Decision

After completing Sprint 2-4 (routing, webhooks, reliability), evaluate whether to evolve toward the RFC-003 vision of an "Agentic Loop Runtime" or mature as an automation tool.

## Context

RFC-003 proposes evolving senechal-gw from a scheduled automation gateway into an execution runtime for agent-driven workflows. The core thesis: the scheduler tick IS an agentic loop (observe → decide → act → record → repeat), and we can generalize this pattern.

Key insight: "Better personal automation" and "agent execution runtime" are overlapping goals, not competing ones.

## Acceptance Criteria

After Sprint 4 completion, assess:
- **Usage patterns:** Are you using it for real automation? What patterns emerge?
- **Workflow need:** Is event routing sufficient or do multi-step workflows feel necessary?
- **Agent demand:** Are external agents actually requesting to use it, or is it personal-only?
- **Complexity trade-off:** Does RFC-003 add enough value to justify the architectural evolution?

Based on assessment, choose one path:
- **Option A:** Mature automation tool (stop at Sprint 4, focus on stability/polish)
- **Option B:** Add workflow engine (Sprint 5: YAML workflows, multi-step chains)
- **Option C:** Full RFC-003 (Sprint 5-7: workflows + run model + agent API)

## RFC-003 Key Concepts (For Reference)

**What would change:**
- **Events:** First-class abstraction (not just scheduler side-effect)
- **Skills:** Richer capability manifests (vs plugins)
- **Workflows:** Declarative YAML chains with variables/conditionals
- **Runs:** Execution instances with run_id, context, ledger
- **Agent API:** `POST /run/{workflow}` HTTP endpoint
- **Ledger:** Append-only execution history for replayability

**What stays the same:**
- Subprocess execution model
- State persistence
- Serial FIFO dispatch
- Protocol v1 JSON I/O

**Sprint 2-4 are prerequisites regardless:** Event routing, webhooks, and reliability features are needed for both paths.

## Evaluation Questions

1. **Workflow complexity:** Do real use cases need multi-step sequences, or is single-plugin + routing sufficient?
2. **Agent integration:** Is there actual demand for external agents to invoke workflows via API?
3. **Observability needs:** Is job_log sufficient or do we need full run ledger with step tracking?
4. **Declarative vs code:** Do YAML workflows feel natural or over-engineered for target use cases?
5. **Positioning:** Is "personal automation tool" or "agentic runtime" the right market?

## Success Criteria for "Proceed to RFC-003"

Proceed with RFC-003 evolution (Sprint 5+) if:
- ✅ You have 3+ real use cases that need multi-step workflows
- ✅ External agents/tools are requesting programmatic access
- ✅ Observability/replayability is blocking real usage
- ✅ You're excited about building an execution runtime, not just automation

Otherwise: Declare Sprint 4 as "1.0", focus on stability, plugins, and real-world usage.

## Decision Point

After Sprint 4:
- Review this card
- Assess actual vs hypothetical needs
- Choose evolution path
- Update roadmap accordingly

No commitment needed now. Sprint 2-4 work is valuable regardless of direction.

## Landscape Context

RFC-003 positions senechal-gw in a space between:
- **Cron/systemd timers** (too simple, no state)
- **Airflow/Temporal** (too complex, distributed)
- **n8n/Zapier** (SaaS, not self-hosted)
- **LangChain/AutoGPT** (LLM-coupled, not deterministic)

The "agentic loop runtime" niche: deterministic execution kernel that agents can use but don't require.

## Reference

- RFC-003: `/Volumes/Projects/senechal-gw/RFC-003-Agentic-Loop-Runtime.md`
- Current roadmap: Sprint 2 (routing), Sprint 3 (webhooks), Sprint 4 (reliability)

## Narrative

- 2026-02-09: Card created to defer RFC-003 architectural decision until after Sprint 4. Sprint 2-4 are prerequisites regardless of direction. Goal is to avoid distraction while acknowledging the landscape and far-away possibilities. "senechal tick, is a loop" - the scheduler already implements an observe-decide-act-record pattern; RFC-003 just generalizes it. Decision deferred until we have real usage data and can evaluate workflow complexity vs routing simplicity trade-offs. (by @claude)
