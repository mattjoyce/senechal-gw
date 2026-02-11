---
id: 22
status: done
priority: High
blocked_by: []
tags: [sprint-3, epic, webhooks, security]
---

# Sprint 3: Webhooks + Security + Observability

Implement webhooks, token-based authorization with manifest-driven scopes, multi-file configuration, and real-time event streaming. Foundation for secure 3rd party integrations.

## Scope

**Core Features:**
1. Multi-file configuration system (Nagios-style)
2. Plugin manifest command type metadata (read vs write)
3. Token scopes with BLAKE3 integrity checking
4. SSE /events endpoint for real-time observability
5. Webhook HTTP listener with HMAC verification
6. /healthz endpoint

**Foundation:** This sprint enables secure webhook integrations with 3rd parties. Token scopes prevent over-permissioned external services. Multi-file config enables LLM-safe editing.

## Implementation Cards

**Prerequisites (Sprint 2 completion):**
- #29 - Job Storage + Auth (MUST complete before Sprint 3)

**Sprint 3 Cards (in implementation order):**

1. **#39 - Multi-File Config System** (foundation)
   - Priority: High
   - Blockers: None
   - `~/.config/senechal-gw/` with separate YAML files
   - BLAKE3 hash verification on scope files
   - Compile-time validation

2. **#36 - Manifest Command Type Metadata**
   - Priority: High
   - Blockers: None
   - Add `type: read|write` to manifest commands
   - Backward compatible (array format deprecated Sprint 5)

3. **#35 - Token Scopes**
   - Priority: High
   - Blockers: #36
   - Manifest-driven scopes (`plugin:ro`, `plugin:rw`)
   - Low-level scopes (`action:resource:command`)
   - Explicit deny rules

4. **#33 - SSE /events Endpoint**
   - Priority: High
   - Blockers: None
   - Real-time event stream for debugging
   - Job state transitions, scheduler ticks, plugin lifecycle
   - Enables TUI monitor (#34)

5. **Webhook Listener + HMAC**
   - Priority: High
   - Blockers: #39 (multi-file config)
   - POST /webhook/{path} with HMAC-SHA256 verification
   - Body size limits, error handling
   - Enqueues jobs for configured plugin

6. **/healthz Endpoint**
   - Priority: Normal
   - Blockers: None
   - System status, uptime, queue depth, circuit breakers
   - No authentication required (localhost only recommended)

**Optional (nice-to-have):**
- #34 - TUI Monitor (depends on #33)
- #37 - Config Doctor (validation tool, could slip to Sprint 4)

## Acceptance Criteria

**Multi-File Config (#39):**
- Config loaded from `~/.config/senechal-gw/` directory
- Separate files: config.yaml, plugins.yaml, routes.yaml, webhooks.yaml, tokens.yaml
- BLAKE3 hash verification on scope files (hard fail if mismatch)
- Cross-file reference validation (routes reference valid plugins, etc.)

**Manifest Metadata (#36):**
- Manifest supports both array and object command formats
- Object format includes `type: read|write` per command
- Validation enforces valid type values
- Plugin registry provides `GetReadCommands()`, `GetWriteCommands()`

**Token Scopes (#35):**
- Tokens defined in tokens.yaml with scope file references
- Scope expansion: `plugin:ro` → only type:read commands
- Authorization middleware checks scopes on all API requests
- 403 with minimal error on insufficient scope

**SSE /events (#33):**
- GET /events streams Server-Sent Events
- Event types: job.*, scheduler.*, plugin.*, router.*, webhook.*
- Ring buffer for late-joining clients
- Authentication via bearer token

**Webhooks:**
- POST /webhook/{path} endpoints from webhooks.yaml
- HMAC-SHA256 signature verification (mandatory)
- 403 on invalid signature with no details
- Body size limit enforced (default 1MB)
- Enqueues job for configured plugin:command

**/healthz:**
- GET /healthz returns JSON status
- Fields: uptime, queue_depth, plugins_loaded, circuit_breakers
- No authentication (localhost recommended)

## Testing

**Integration Test Flow:**
1. Initialize multi-file config directory
2. Create token with scoped access (`github-webhook:rw`, `read:jobs`)
3. Configure webhook endpoint with HMAC secret
4. Send test webhook with valid HMAC → job enqueued
5. Query GET /job/{id} with scoped token → status returned
6. Query GET /events with token → receive job.* events
7. Send webhook with invalid HMAC → 403
8. Try POST /trigger/withings/poll with limited token → 403

## Dependencies

**External:**
- BLAKE3 library: `github.com/zeebo/blake3`
- SSE handling: Standard library sufficient

**Internal:**
- Sprint 2 completion (especially #29 auth helpers)
- Existing plugin discovery (#12)
- Existing queue + state (#4, #5, #7)

## Deferred to Sprint 4

- #38 - CLI Config Management (nice-to-have, not blocking)
- #37 - Config Doctor (can use manual validation for now)
- Event routing (plugin chaining) - still in Sprint 2 backlog (#21)

## Success Metrics

- GitHub webhook integration working with scoped token
- Token scope prevents unauthorized plugin access (403s logged)
- Multi-file config edited by LLM without corruption
- /events stream shows real-time job state for debugging
- BLAKE3 hash mismatch detected and prevents startup

## Narrative
- 2026-02-10: All Sprint 3 work completed and merged. Multi-file config + webhooks (PR #11), token scopes + SSE + healthz (PR #12), user guide updates (PR #13). (by @team)

