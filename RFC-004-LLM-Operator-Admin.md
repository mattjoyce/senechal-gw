# RFC-004: LLM as Operator/Admin Model for Seneschal Gateway

Status: Draft\
Author: Matt Joyce\
Project: ductile\
Date: 2026

------------------------------------------------------------------------

# 1. Abstract

This RFC defines the design principles and architectural direction for
treating a Large Language Model (LLM) as a first-class operator and
administrative interface for the Seneschal Gateway.

Rather than positioning the system immediately as a fully autonomous
agent runtime, this proposal frames Seneschal as a **boundary gateway**
that:

-   isolates execution risk
-   exposes structured capabilities ("Skills")
-   enables controlled LLM-driven orchestration
-   supports gradual evolution toward agentic behaviour without
    requiring immediate autonomy.

The goal is to create a **self-configuring operator loop** where an LLM
can safely inspect, plan, and execute actions within explicit
guardrails.

------------------------------------------------------------------------

# 2. Motivation

Modern agent frameworks often collapse reasoning, orchestration, and
execution into a single opaque system.

This introduces risks:

-   reduced observability
-   uncontrolled capability access
-   unclear permission boundaries
-   operational fragility.

Instead, this architecture introduces a separation:

LLM → Seneschal Gateway → Skills → External Systems

Where:

-   the LLM acts as operator/admin
-   the gateway acts as safety boundary
-   skills represent controlled execution units.

------------------------------------------------------------------------

# 3. Vision

The LLM becomes:

-   Operator
-   Planner
-   Inspector
-   Controlled executor

The gateway becomes:

-   policy enforcement boundary
-   capability registry
-   execution ledger
-   safety envelope.

This allows gradual evolution toward a self-configuring agent while
maintaining deterministic governance.

------------------------------------------------------------------------

# 4. Core Principles

## 4.1 LLM as First-Class User

The system must treat LLM interaction as a primary interface, not an
add-on.

Implications:

-   structured outputs
-   predictable APIs
-   deterministic result envelopes
-   clear affordances.

## 4.2 Boundary Isolation

The gateway isolates:

-   credentials
-   system state
-   execution risk
-   side effects.

LLMs never directly execute arbitrary system commands.

## 4.3 Skills as Capabilities

All executable functions are exposed as Skills.

Characteristics:

-   manifest-defined
-   schema-driven inputs/outputs
-   explicit side-effect declarations
-   subprocess isolation.

## 4.4 Deterministic First

Agentic autonomy is deferred.

Initial design prioritizes:

-   deterministic workflows
-   operator-driven execution
-   explicit planning loops.

------------------------------------------------------------------------

# 5. Conceptual Model

This model distinguishes between:

-   **Operator utilities** (administrative functions of the gateway
    itself)
-   **Skills** (runtime-executable capabilities exposed to an LLM
    operator)

This RFC uses "utility" specifically to mean *gateway-admin functions*
(validate/configure/inspect), not plugins or actions.

## 5.1 Seneschal Operator Utilities (Admin Tooling)

Seneschal should provide first-class administrative utilities that allow
an LLM (and humans) to operate the system safely.

These utilities are *not* plugins and do not represent external actions;
they are gateway-local affordances to manage integrity, configuration,
and registration.

Examples:

-   validate configuration (syntax + schema + policy checks)
-   list registered plugins / skills
-   register / deregister plugins (or enable/disable)
-   recompute and verify hashes (config, manifests, plugin binaries)
-   show effective policy (resolved permissions)
-   run "doctor" checks (filesystem layout, permissions, secrets
    availability)
-   show the "capability graph" / index
-   inspect last N runs / ledger summaries (read-only).

Implementation note:

-   These utilities may be exposed as subcommands of the main binary
    (e.g., `ductile validate`, `ductile plugins list`,
    `ductile skills export`).
-   Alternatively, they may be separate binaries, but the preference is:
    **single binary, subcommands**.

## 5.2 Skills

Skills are generated from plugin manifests and represent controlled
execution units.

They are:

-   operator-facing affordances
-   LLM-readable capability descriptions
-   executed via subprocess isolation
-   governed by explicit permissions and side-effect declarations.

Skill documentation includes:

-   purpose
-   inputs
-   outputs
-   side effects
-   risk tier.

## 5.3 Guardrails

Guardrails define safety constraints:

-   permission tiers (READ / WRITE / DANGEROUS)
-   dry-run support for mutations
-   validation before execution
-   state patch constraints.

## 5.4 Affordances

Affordances describe how the LLM can interact with the system.

Examples:

-   inspect capabilities
-   plan execution
-   request dry-run
-   execute approved actions
-   review ledger history
-   invoke operator utilities for validation and safe administration.

------------------------------------------------------------------------

# 6. Interaction Model

Example operator loop:

1)  LLM queries capability registry (skills index).
2)  LLM selects skill.
3)  LLM performs dry-run.
4)  Gateway returns structured preview.
5)  LLM requests execution.
6)  Gateway enforces permissions and executes.
7)  Result stored in execution ledger.

Operator utilities are used adjacent to this loop to keep the system
safe and coherent:

-   validate config before applying changes
-   verify plugin hashes
-   confirm effective permissions.

------------------------------------------------------------------------

# 7. Result Envelope

All actions return:

-   status
-   summary
-   structured output
-   artifacts
-   warnings
-   run_id.

This ensures predictable operator reasoning.

------------------------------------------------------------------------

# 8. Permission Model

Skills are classified into tiers:

READ: - inspection only - no side effects.

WRITE: - reversible or bounded side effects.

DANGEROUS: - destructive or high-risk actions - require elevated
confirmation.

------------------------------------------------------------------------

# 9. Skill Documentation (Skill.md)

Skill.md serves as:

-   operator manual
-   LLM capability reference.

Each skill includes:

-   name
-   tier
-   description
-   inputs
-   outputs
-   side effects
-   examples
-   failure modes.

Future direction:

Skill.md generated from manifests to prevent drift.

------------------------------------------------------------------------

# 10. Self-Configuring Agent Direction

Long-term vision:

-   LLM uses introspection skills and operator utilities to adjust
    workflows and configurations
-   gateway maintains governance boundary
-   autonomous configuration occurs within constrained policy and
    validation affordances.

------------------------------------------------------------------------

# 11. Risks and Unknowns

-   permission escalation complexity
-   prompt ambiguity impacting execution decisions
-   state evolution boundaries
-   balancing autonomy with control
-   operator utilities becoming an unintended "escape hatch" if not
    constrained and audited.

------------------------------------------------------------------------

# 12. Non-Goals (Current Phase)

-   fully autonomous agent loops
-   uncontrolled tool execution
-   replacing deterministic workflows with free-form tool selection.

------------------------------------------------------------------------

# 13. Conclusion

Treating the LLM as operator/admin enables:

-   safe agentic evolution
-   deterministic governance
-   capability-centric architecture.

The gateway becomes the controlled interface where intention is
translated into action under explicit guardrails, supported by operator
utilities that maintain system integrity.
