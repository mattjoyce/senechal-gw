# RFC-003: Agentic Loop Runtime

**Status:** Draft (partially superseded by RFC-004 and RFC-006)
**Author:** Matt Joyce
**Project:** ductile
**Date:** 2026

------------------------------------------------------------------------

## 1. Abstract

This RFC proposes evolving the current ductile project from a
scheduled automation gateway into an **Agentic Loop Runtime**.

The Agentic Loop Runtime is a lightweight execution kernel designed to
orchestrate workflows composed of declarative capabilities ("skills").
It provides a deterministic execution environment that is:

-   event-driven rather than schedule-centric
-   capability-oriented rather than tool-script driven
-   agent-compatible without requiring LLM dependence

Cron-based scheduling remains a supported trigger but is no longer the
primary architectural abstraction.

------------------------------------------------------------------------

## 2. Motivation

Traditional automation systems operate under one of two paradigms:

1)  Time-driven scheduling (cron)
2)  Static integration flows

Emerging AI systems introduce a third paradigm:

> Agentic execution loops where intention, capability selection, and
> execution occur iteratively.

Current agent frameworks often:

-   tightly couple reasoning and execution
-   lack deterministic observability
-   obscure operational control

This project seeks to establish:

-   deterministic execution
-   inspectable state
-   composable capabilities
-   LLM-optional agentic affordances

------------------------------------------------------------------------

## 3. Core Thesis

The Agentic Loop Runtime acts as an execution kernel where:

Event → Workflow → Skill Chain → Result Payload → Ledger Entry

Rather than building an "AI agent", the runtime provides structured
affordances enabling agents to operate safely.

------------------------------------------------------------------------

## 4. Conceptual Model

### 4.1 Events

Events trigger workflow execution.

Supported event types include:

-   Manual invocation (API)
-   Scheduled triggers (cron-like)
-   Webhooks
-   External agent requests

Cron becomes one event source among many.

### 4.2 Skills (Capability Abstraction)

Skills are executable capabilities derived from plugin manifests.

Characteristics:

-   implemented as subprocesses
-   language agnostic
-   isolated execution boundary
-   declared input/output schema
-   declared side effects

### 4.3 Workflows

Workflows define chains of skills.

Characteristics:

-   declarative (YAML-based)
-   deterministic execution order
-   reusable composition

### 4.4 Runs

A Run represents an execution instance.

Each run includes:

-   unique run_id
-   execution context
-   input payload
-   output payload
-   execution ledger record

------------------------------------------------------------------------

## 5. Execution Envelope

Plugins receive structured input:

-   run context
-   workflow variables
-   durable state
-   direct inputs

Plugins emit structured output including:

-   result payload
-   variable updates
-   state patches

Plugins do not directly mutate storage; the runtime applies validated
state updates.

------------------------------------------------------------------------

## 6. State Model

### 6.1 Run Context (Ephemeral)

Per execution:

-   inputs
-   temporary variables
-   execution metadata

### 6.2 Workflow State (Durable)

Persisted between runs:

-   cursors (e.g., last processed timestamp)
-   deduplication markers
-   minimal workflow memory

State is stored schemaless (NoSQL-style) but bounded by envelope
structure and patch constraints.

### 6.3 Resource State (Future)

Optional durable records describing external resources.

------------------------------------------------------------------------

## 7. Execution Ledger

The runtime maintains an append-only ledger capturing:

-   run start/end
-   steps executed
-   timing
-   outputs
-   errors

Purpose:

-   replayability
-   observability
-   auditability
-   agent introspection

------------------------------------------------------------------------

## 8. Capability Registry

Skill manifests are transformed into Skills.

Skills represent agent-visible capabilities:

-   description
-   inputs
-   outputs
-   constraints
-   side effects

The registry enables deterministic workflow design and future
agent-driven skill selection.

------------------------------------------------------------------------

## 9. Agent Interaction Model

Agents interact via:

POST /run/{workflow}

Agents receive:

-   result payload
-   artifacts
-   execution summary

Initial implementation assumes deterministic workflows.

------------------------------------------------------------------------

## 10. Initial Use Case

Daily event:

-   detect newly created public GitHub repositories
-   generate structured write-up
-   create note within Obsidian vault

Demonstrates:

-   state cursor usage
-   idempotent execution
-   artifact generation

------------------------------------------------------------------------

## 11. Architectural Principles

-   Deterministic first
-   Agent-ready, not agent-dependent
-   Explicit state boundaries
-   Capability isolation via subprocesses
-   Declarative workflows
-   Observable execution

------------------------------------------------------------------------

## 12. Evolution Path

Phase 1:

-   Event-driven execution
-   Workflow chaining
-   Skill manifests

Phase 2:

-   Capability registry
-   agent-triggered runs

Phase 3:

-   conditional logic
-   decision nodes

Phase 4:

-   adaptive agentic loops

------------------------------------------------------------------------

## 13. Risks and Unknowns

-   state schema evolution
-   capability permissioning
-   scaling execution concurrency
-   governance of agent-triggered actions

------------------------------------------------------------------------

## 14. Open Questions

-   should skill selection be deterministic or agent-mediated?
-   how should policy enforcement integrate?
-   how should long-running workflows be resumed?

------------------------------------------------------------------------

## 15. Alternatives Considered

-   pure cron-based automation
-   full agent frameworks
-   traditional integration engines

------------------------------------------------------------------------

## 16. Conclusion

The Agentic Loop Runtime establishes a foundational execution layer
enabling deterministic automation today while preparing for agent-driven
orchestration tomorrow.
