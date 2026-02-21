# Ductile Development Team

## Team Roster

**Team Lead:** Claude (coordination, architecture decisions)

**Agent 1 (Claude):** Configuration & Plugin System
**Agent 2 (Codex):** State & Queue Management
**Agent 3 (Gemini):** Observability & Orchestration

## Branch Naming Convention

**IMPORTANT:** All branches MUST be prefixed with agent codename:
- Agent 1 (Claude): `claude/*`
- Agent 2 (Codex): `codex/*`
- Agent 3 (Gemini): `gemini/*`

Examples: `claude/dispatch`, `codex/metrics`, `gemini/scheduler-integration`

## Agent Capability Assessment

Based on Sprint 1 & 2 execution:
- **Agent 1 (Claude):** Complex multi-component work, strong architecture understanding
- **Agent 2 (Codex):** Solid implementation, good with focused tasks and testing
- **Agent 3 (Gemini):** Produces high-quality code, best with well-defined single-component tasks

## Completed Sprints

| Sprint | Goal | PRs | Status |
|--------|------|-----|--------|
| Sprint 1 (Phases 0-3) | MVP Core Loop - config, queue, state, scheduler, dispatch, echo plugin, main wiring | #4, #5, #6 | Complete |
| Sprint 2 | API Triggers - HTTP server, job storage, auth, user guide | #8, #9, #10 | Complete |

---

## Sprint 3: Webhooks + Security + Observability (Current)

**Goal:** Secure 3rd party webhook integrations with token-based auth and real-time observability

**Foundation:** Multi-file config enables LLM-safe editing. Token scopes prevent over-permissioned access. SSE events enable real-time debugging.

### Agent 1 (Claude): Multi-File Config + Webhooks
- **Branch:** `claude/sprint3-config-webhooks`
- **Cards:** #39, #42
- **Deliverable:** Multi-file configuration system + webhook endpoints
- **Work:**
  - #39: Multi-file config system (`~/.config/ductile/` directory structure)
  - BLAKE3 hash verification on scope files
  - Cross-file reference validation
  - #42: Webhook listener with HMAC-SHA256 verification (after #39)
  - POST /webhook/{path} endpoints from webhooks.yaml
  - Body size limits, job enqueueing
- **Dependencies:** None (foundation work)

### Agent 2 (Codex): Metadata + Auth + Observability
- **Branch:** `codex/sprint3-metadata-auth-obs`
- **Cards:** #36, #35, #33, #43
- **Deliverable:** Manifest metadata + token auth + observability endpoints
- **Work:**
  - #36: Manifest command type metadata (read vs write)
  - #35: Token scopes with manifest-driven permissions (after #36, needs #39)
  - #33: SSE /events endpoint for real-time debugging
  - #43: /healthz endpoint for monitoring
- **Dependencies:** #36 blocks #35, #35 needs #39 from Agent 1

### Merge Order
1. **Agent 2** (codex/sprint3-metadata-scopes) - #36 first, then rest
2. **Agent 1** (claude/sprint3-config-webhooks) - #39 first, then #42
3. **Agent 3** (Gemini) - Documentation after Sprint 3 dev work merged

---

## Future Sprints

### Sprint 4: Reliability Controls
- Circuit breaker (auto-pause failing plugins)
- Retry with exponential backoff
- Deduplication enforcement
- Event routing (plugin chaining)

### Post-Sprint 4: RFC-003 Evaluation
Card #27 - Assess evolution toward "Agentic Loop Runtime" or mature as automation tool.
Decision deferred until real usage data available.

See `kanban/rfc-003-evaluation.md` and `RFC/RFC-003-Agentic-Loop-Runtime.md` for details.

---

## Agent Development Workflow

**Simple Rule: One Card → One PR → Merge → Next Card**

Never work on multiple cards in the same branch.

### Process

1. **Get card assignment** — you will be assigned one card at a time
2. **Create branch from main:**
   ```bash
   git checkout main && git pull origin main
   git checkout -b claude/card39-multi-file-config
   ```
   Branch naming: `<agent-name>/card<number>-<short-description>`
3. **Read the card** thoroughly
4. **Do the work** — implement, write tests, ensure `go test ./...` passes
5. **Update the card** — set `status: done`, add narrative entry
6. **Push + create PR immediately:**
   ```bash
   git push -u origin claude/card39-multi-file-config
   gh pr create --title "config: implement multi-file config system (#39)" --body "..."
   ```
7. **WAIT for merge** — do not start the next card until PR is merged
8. **Get next card** — clean up branch, pull main, repeat

### Commit Format

```
<component>: <action> <what>

Implements #39
```

**Component prefixes:** `config:` `state:` `queue:` `scheduler:` `dispatch:` `plugin:` `webhook:` `router:` `cli:` `protocol:` `docs:` `test:` `chore:`

### Rules

- One card per branch, one PR per card
- Don't start next card before previous PR merges
- Don't push without creating a PR
- Always update card status and add narrative
- Fix tests before creating PR — never PR with failing tests
- Bugs in your card's scope: fix them. Outside scope: report, don't fix.
- If blocked by another card: tell coordination immediately

## Decision Log

- **Crash Recovery (ID 24):** Option B — re-queue if under `max_attempts` (SPEC semantics)
