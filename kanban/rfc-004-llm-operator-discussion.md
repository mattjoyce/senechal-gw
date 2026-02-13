---
id: 41
status: backlog
priority: High
blocked_by: [22]
tags: [sprint-5, planning, architecture, rfc, multi-agent]
---

# RFC-004: LLM as Operator/Admin Model - Design Discussion

Multi-agent design review session for RFC-004 with Claude, Gemini, and Codex. Review architectural direction for treating LLM as first-class operator/admin interface.

## Purpose

**RFC Location:** `/Volumes/Projects/ductile/RFC-004-LLM-Operator-Admin.md`

**Core Proposal:**
- Treat LLM as first-class operator/admin (not just API consumer)
- Seneschal Gateway as safety boundary between LLM and external systems
- Skills as controlled execution units derived from plugin manifests
- Operator utilities for gateway administration
- Gradual evolution toward agentic behavior with guardrails

**Architecture:**
```
LLM → Seneschal Gateway → Skills → External Systems
```

## Discussion Topics

### 1. Skills Design
**Question:** What is the right granularity for Skills?

**Options:**
- **A:** 1 skill = 1 plugin command (`withings:poll`)
- **B:** Skills = higher-level compositions (`fetch-health-data` executes multiple plugins)
- **C:** Hybrid approach (both primitive and composite skills)

**Current state:** Plugin commands already exist. Skills would be a layer on top.

**Implications:**
- Manifest extensions needed (tier, side_effects, dry_run_supported)
- Skills registry/index generation
- Documentation generation (Skill.md per skill)

### 2. Operator Utilities Security
**Question:** Should LLMs have direct access to operator utilities?

**Utilities include:**
- `ductile config token create/delete`
- `ductile config plugin set`
- `ductile config route add/remove`
- `ductile config webhook add`
- `ductile doctor`
- `ductile config token rehash`

**Options:**
- **A:** Human-only (CLI only, no API)
- **B:** LLM-accessible with audit (admin:* scope required)
- **C:** Hybrid (LLM can READ, humans WRITE)

**Risk:** "Operator utilities becoming an unintended escape hatch" (RFC §11)

**Example scenario:** Can LLM create token with `admin:*` scope?

### 3. Self-Configuration Boundaries
**Question:** What config changes can LLM make autonomously vs requiring human approval?

**Proposed tiers:**
- **READ:** Inspect plugins, tokens, routes, ledger
- **WRITE-BOUNDED:** Add route (if plugins exist), adjust schedule (within bounds)
- **DANGEROUS:** Create token, add webhook, modify credentials

**Implementation approach:**
- Token scopes enforce boundaries
- Approval flow API for DANGEROUS tier
- Audit all LLM-initiated changes

**Key decision:** Where to draw the autonomy line?

### 4. Manifest Extensions for Skills
**Question:** What fields should manifests include to support Skills model?

**Proposed additions:**
```yaml
commands:
  poll:
    type: read                    # Existing (Sprint 3)
    tier: READ                    # NEW: Permission tier
    description: "..."            # Existing
    side_effects:                 # NEW: Explicit declarations
      - "updates plugin state"
    dry_run_supported: false      # NEW: Can plugin simulate?
    examples:                     # NEW: For LLM context
      - input: {}
        output: {"weight_kg": 75.2}
    failure_modes:                # NEW: Known error conditions
      - "API rate limit exceeded"
      - "OAuth token expired"
```

**Questions:**
- Auto-infer tier from type (read → READ, write → WRITE)?
- Or require explicit tier declaration?
- How detailed should side_effects be?
- Should examples be in manifest or separate Skill.md?

### 5. Dry-Run Implementation Strategy
**Question:** How should dry-run be implemented?

**Phase 1 (MVP):** Opt-in per plugin
- Plugin declares `dry_run_supported: true`
- Gateway passes `dry_run: true` in request
- Plugin simulates and returns preview

**Phase 2:** Gateway-level simulation
- For simple plugins, gateway previews without exec
- Based on manifest declarations

**Phase 3:** Advanced plugin support
- Plugins implement sophisticated dry-run logic
- Return detailed impact analysis

**Decision needed:** Which phase for Sprint 5?

### 6. Execution Ledger Design
**Question:** Extend job storage or create separate audit log?

**Option A: Extend jobs table**
```sql
ALTER TABLE jobs ADD COLUMN invoked_by TEXT;    -- "human", "llm-claude", "cron"
ALTER TABLE jobs ADD COLUMN approved_by TEXT;   -- For human-in-loop
ALTER TABLE jobs ADD COLUMN dry_run BOOLEAN;
```

**Option B: Separate ledger**
```sql
CREATE TABLE execution_ledger (
    id INTEGER PRIMARY KEY,
    job_id TEXT REFERENCES jobs(id),
    invoked_by TEXT,
    skill_name TEXT,
    tier TEXT,
    approved_by TEXT,
    timestamp DATETIME
);
```

**Trade-offs:** Simplicity vs separation of concerns

### 7. Sprint 5 Scope
**Question:** What is the minimal viable Skills implementation?

**Candidates for Sprint 5:**
- [ ] Extend manifest schema (tier, side_effects, dry_run)
- [ ] Skills registry generator (from manifests)
- [ ] Skills.md documentation generator
- [ ] Execution ledger (extend jobs table)
- [ ] LLM operator token example config
- [ ] Dry-run protocol (Phase 1: opt-in)
- [ ] Approval flow API (for DANGEROUS tier)

**Which subset is MVP?**

## Pre-Discussion Preparation

**Each agent should review:**
1. RFC-004 document (`/Volumes/Projects/ductile/RFC-004-LLM-Operator-Admin.md`)
2. Current Sprint 3 implementation (#35, #36, #39)
3. Existing manifest format (SPEC.md §5.4)
4. Token scopes design (#35)

**Each agent should prepare:**
- Position on Skills granularity
- Opinion on operator utilities security
- Proposed manifest extensions
- Sprint 5 scope recommendation

## Success Criteria

**At end of discussion:**
- ✅ Consensus on Skills model (primitive vs composite)
- ✅ Decision on operator utilities access (human-only vs LLM-accessible)
- ✅ Agreement on self-configuration boundaries
- ✅ Finalized manifest schema extensions
- ✅ Sprint 5 scope defined (MVP features)
- ✅ Implementation cards created for Sprint 5

## Dependencies

- Sprint 3 completion (#22) - Need multi-file config, token scopes, manifest metadata in place
- RFC-004 document finalized - Any open questions resolved before discussion

## Output Artifacts

**Expected outputs from discussion:**
1. **RFC-004 Decisions Document** - Captures consensus on open questions
2. **Updated SPEC.md** - §5.4 (Manifest) extended with Skills fields
3. **Sprint 5 Implementation Cards** - Specific tasks broken down
4. **Skills.md Template** - Example format for generated documentation

## Notes

**Multi-agent discussion format:**
- Claude: Architecture + security perspective
- Gemini: Developer experience + LLM usability
- Codex: Implementation feasibility + technical constraints

**Facilitation:**
- Matt Joyce moderates
- Each agent presents position on each topic
- Consensus-building on key decisions
- Document decisions with rationale

## Follow-Up

After discussion:
- Update RFC-004 with decisions
- Create Sprint 5 cards
- Update SPEC.md
- Archive this card as done

## References

- RFC-004: `/Volumes/Projects/ductile/RFC-004-LLM-Operator-Admin.md`
- SPEC.md §5.4: Plugin Manifest
- Card #35: Token Scopes
- Card #36: Manifest Metadata
- Card #39: Multi-File Config

## Narrative

This discussion determines the future direction of Seneschal as an LLM-operator platform. The decisions made here will guide Sprint 5+ implementation and shape whether Seneschal becomes a truly differentiated agent runtime with built-in safety boundaries.

The timing (after Sprint 3) is intentional—we need the foundational pieces (multi-file config, token scopes, manifest metadata) in place before designing the Skills layer on top.
