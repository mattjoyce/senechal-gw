# RFC-006: AgenticLoop + Ductile Federated Topology

**Status:** Active Draft
**Date:** 2026-02-21
**Author:** Matt Joyce
**Depends on:** RFC-001, RFC-002-Decisions

---

## Summary

Adopt a federated execution model:

1. AgenticLoop remains the planner/operator runtime.
2. Ductile remains the execution boundary.
3. Run a local Ductile instance per managed host for host-local admin actions.
4. Keep one boundary Ductile as the north-south policy and mediation layer.

This preserves least privilege while keeping one tool contract for the agent.

---

## Problem

We currently have two tool surfaces:

- AgenticLoop-native tools (planner-facing)
- Ductile plugins (execution-facing)

Without a clear boundary, privileged operations (filesystem, service admin, package management, container operations) can end up routed through the wrong trust domain.

---

## Goals

- Define a single operational model for local and remote actions.
- Minimize blast radius for host-level administration tasks.
- Preserve policy enforcement at a central boundary.
- Keep plugin/tool contracts composable across environments.
- Provide a deployment shape that can be scripted.

## Non-Goals

- Replace AgenticLoop runtime semantics.
- Merge both codebases into one binary.
- Specify full production hardening for every environment in this RFC.

---

## Decision

Use a two-tier Ductile topology:

1. **Boundary Ductile (Control Plane)**
- Internet/API-facing boundary.
- Token/auth policy, auditing, routing, and mediation.
- May proxy requests to host-local Ductile instances.

2. **Host-Local Ductile (Execution Plane)**
- Runs on each managed node (Unraid, Linux server, workstation, etc.).
- Executes local admin plugins close to resources and permissions.
- Exposes only trusted interfaces to the boundary plane.

3. **AgenticLoop Integration Rule**
- AgenticLoop should call Ductile APIs, not raw host tools.
- For mutating host actions, route to that host's local Ductile.
- For cross-system orchestration, route via boundary Ductile.

---

## Execution Classification

### Must run on host-local Ductile

- Service lifecycle (`systemctl`, daemon config, restarts)
- Filesystem mutations
- Package/runtime install/update
- Container runtime operations
- Host networking/firewall changes

### May run on boundary Ductile

- Capability discovery (`/plugins`, `/openapi.json`)
- Cross-host orchestration and fan-out
- External API mediation and normalization
- Read-only coordination queries

### Default rule

If an action changes host state, run it on that host-local Ductile.

---

## Reference Flow

1. AgenticLoop plans action chain.
2. AgenticLoop invokes boundary Ductile endpoint.
3. Boundary policy resolves target host/trust domain.
4. Boundary forwards to host-local Ductile when action is host-scoped.
5. Host-local Ductile enqueues/executes plugin and returns job status.
6. Boundary aggregates response and returns normalized result.

---

## Security Model

- Treat each local Ductile as a separate trust domain.
- Use scoped tokens per plane (no shared global admin token).
- Keep host credentials local to that host's Ductile config.
- Require explicit allowlist of callable plugins per caller identity.
- Preserve immutable audit trail at both boundary and local layers.

---

## Deployment Model (Draft)

### Topology

- `ductile-boundary` (always-on gateway)
- `ductile-node-<hostname>` per managed host

### Deployment script targets

1. Bootstrap local Ductile service on host.
2. Install/register host-local plugin set.
3. Generate scoped service tokens.
4. Register host in boundary routing map.
5. Verify health and discovery endpoints.
6. Run a smoke pipeline (`health -> noop local action -> status fetch`).

### Minimum script outputs

- Host registration record (host ID, base URL, scopes)
- Generated token refs (not secret values in git)
- Health check report
- Plugin discovery snapshot

---

## Config Conventions (Proposed)

- Stable host IDs: `site.role.hostname` (example: `home.unraid.tower`)
- Boundary route hint in plugin metadata: `execution_domain: local|boundary`
- Shared correlation ID propagated across planes for traceability

---

## Migration Plan

1. Inventory AgenticLoop tools and Ductile plugins by capability.
2. Classify capabilities as `local` vs `boundary`.
3. Implement boundary forwarding adapters for selected host-local actions.
4. Move high-risk admin actions to host-local first.
5. Decommission duplicated direct-tool paths once parity is proven.

---

## Open Questions

- Should boundary-to-local forwarding be pull (agent on host) or push (direct API call)?
- How should we express `execution_domain` in manifests without overfitting v1?
- Do we require mTLS between boundary and local Ductile in v1, or token + network ACL?
- What is the minimum offline behavior when boundary is unavailable but local admin is needed?

---

## Acceptance Criteria for Follow-On Work

- Deployment scripts can provision one boundary and at least one local node.
- A host-local admin action executes without exposing host credentials to boundary.
- AgenticLoop can call a single API surface while actions route to correct trust domain.
- End-to-end logs correlate one action across planner, boundary, and local node.
