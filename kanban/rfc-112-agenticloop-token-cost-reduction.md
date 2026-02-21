---
id: 112
status: backlog
priority: Medium
blocked_by: []
tags: [agenticloop, llm, cost, tokens, prompt, architecture]
---

# RFC-111: AgenticLoop Token Cost Reduction

## Goal

Reduce prompt token consumption in AgenticLoop runs without degrading agent
reasoning quality or task completion reliability.

## Background

Token profiling of two test runs (2026-02-21) shows that ~95% of cost is
prompt tokens, not completion tokens. The agent generates very little — the
expense is in what gets fed *in* to each stage call.

### Observed costs (gpt-4o-mini, $0.15/1M input · $0.60/1M output)

| Run | Prompt tokens | Completion tokens | Est. cost |
|-----|--------------|-------------------|-----------|
| IP report (2 loops) | 22,149 | 1,219 | ~$0.004 |
| YouTube transcript (2 loops) | 68,582 | 1,625 | ~$0.011 |

### Token breakdown by stage (YouTube run)

| Stage | Prompt | Driver |
|-------|--------|--------|
| Frame ×2 | ~900 | Template + goal |
| Plan ×2 | ~2,150 | + AvailableTools list |
| **Act ×2** | **43,734** | + Frame + Plan + raw tool output (~30k transcript) |
| **Reflect ×2** | **21,804** | + full Act output (transcript echoed back) |

The YouTube transcript (~30k tokens) was fetched in Act step 3, embedded
raw into Act's output, then echoed wholesale into Reflect's `{{.Act}}`
template variable — paying for the same data twice.

## Proposed Changes

### 1. Truncate `{{.Act}}` in the Reflect prompt (highest ROI)

**Problem:** `{{.Act}}` dumps the entire Act stage output into Reflect,
including raw tool responses (transcripts, API payloads, file contents).
Reflect only needs the structured result JSON (`action_taken`, `evidence_paths`,
`result`, `needs_reflect_attention`), not the raw data.

**Proposed fix:** The Act prompt's output contract already asks the agent to
return a compact JSON summary. The runner should pass only that final JSON
object to Reflect via `{{.Act}}`, stripping any preceding tool-call content.

This requires a change in the runner to extract the last JSON block from the
Act step output rather than passing the full raw step content.

**Estimated saving:** ~19k tokens on the YouTube run (~90% of Reflect cost).

---

### 2. Cap raw tool output size before it enters LLM context

**Problem:** Large tool responses (transcripts, web pages, file contents)
are included verbatim in the Act prompt on subsequent iterations and in
the Reflect prompt.

**Proposed fix:** In `DuctileTool` (or the Act executor), enforce a
configurable `max_tool_output_chars` (e.g. 12,000 chars / ~3k tokens).
When a tool response exceeds this threshold:
- Write the full content to `workspace/tool-outputs/<tool>-<step>.txt`
- Pass a truncated excerpt + the workspace path to the LLM context

The agent already has instructions to write evidence to workspace — this
makes large outputs automatically workspace-first.

**New config key:**
```yaml
agent:
  max_tool_output_chars: 12000  # default; 0 = unlimited
```

**Estimated saving:** 25–35k tokens per run with large tool outputs.

---

### 3. Drop AvailableTools from Act on iterations > 1

**Problem:** The full `{{.AvailableTools}}` binding is rendered into every
Act call. After the first iteration the agent has already demonstrated it
knows which tools are available.

**Proposed fix:** Pass `{{.AvailableTools}}` only on iteration 1. On
subsequent iterations, substitute a short placeholder:
```
[Tools unchanged from iteration 1 — use same names and schemas]
```

This is a prompt template change (config-level), not a code change. The
runner would need to expose `{{.Iteration}}` to the Act template, which it
already does.

**Estimated saving:** ~600–1,000 tokens per additional loop iteration.

---

### 4. Shorten verbose prompt boilerplate (minor)

**Problem:** The v2 prompt templates use multi-line XML structure with
detailed `<rules>` and `<output_contract>` blocks. Each stage call carries
~200–400 tokens of structural boilerplate that is repeated every iteration.

**Proposed approach:** Extract shared boilerplate into a single
`<system_rules>` block injected once at the top of the first stage per
loop, rather than repeating it in every stage prompt. Requires a new
template variable, e.g. `{{.IsFirstIteration}}`.

Alternatively: trim the rule lists in the config to the minimum set proven
necessary by testing.

**Estimated saving:** ~300–500 tokens per loop.

---

### 5. (Optional) Stage-level model routing

**Problem:** Frame and Plan are structurally simple tasks (short context,
JSON output). They don't benefit from a stronger model.

**Proposed approach:** Allow per-stage model override in config:
```yaml
llm:
  provider: openai
  model: gpt-4o-mini        # default for act + reflect
  stage_overrides:
    frame: gpt-4.1-nano     # cheapest available
    plan: gpt-4.1-nano
```

**Risk:** Frame quality directly affects Plan and Act. Degrading Frame with
a weaker model could increase loop iterations, potentially costing more
overall. Do not implement without A/B testing loop counts.

**Estimated saving:** 30–50% on Frame/Plan token cost (~900 tokens/run at
lower price tier), but savings are small in absolute terms.

---

## Priority Order

| # | Change | Effort | Risk | Saving |
|---|--------|--------|------|--------|
| 1 | Truncate `{{.Act}}` → Reflect | Medium (runner change) | Low | High |
| 2 | Cap raw tool output size | Medium (runner/tool change) | Low | High |
| 3 | Drop AvailableTools on iter > 1 | Low (prompt + template var) | Low | Low |
| 4 | Shorten prompt boilerplate | Low (config edit) | Low | Low |
| 5 | Stage-level model routing | High (config + runner) | Medium | Low |

Implement 1 and 2 first. They address the same root cause (large data
propagating through the stage pipeline) and together account for >80% of
the excess token spend observed.

## Success Criteria

- [ ] YouTube-class run (with large tool output) costs <20k total tokens
- [ ] IP-report-class run (no large tool output) costs <15k total tokens
- [ ] No regression in task completion rate across existing test runs
- [ ] `max_tool_output_chars` is configurable and documented

## References

- Test runs: `24fefae2` (IP report), `ba1245a2` (YouTube transcript)
- Config: `/home/matt/admin/AgenticLoop-test/config.yaml`
- Token profiling: 2026-02-21 session

## Narrative

- 2026-02-21: RFC created from token profiling of two test runs. Changes 1
  and 2 address the same root cause and should be implemented together.
  (by @claude)
