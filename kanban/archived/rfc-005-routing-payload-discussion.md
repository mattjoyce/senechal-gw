---
id: 47
status: done
priority: High
blocked_by: []
tags: [routing, rfc, design, discussion]
---

# RFC-005: Routing Payload Management Discussion

Review and decide on payload propagation strategy for multi-hop event chains, as described in `RFC-005-ROUTING_OPTIONS.md`.

## Job Story

When plugins emit events that trigger downstream plugins, I want a clear strategy for how context (channel_id, user_id) and data (transcripts, documents) flow through the chain, so that end-of-chain plugins have the information they need without brittle conventions or payload bloat.

## Acceptance Criteria

- Review all 4 options in RFC-005 (baggage forwarding, DB accumulated, hybrid, external refs)
- Decide on MVP approach (RFC recommends Option 1: baggage forwarding)
- Document decision in RFC-005 or SPEC.md
- Identify any mitigations needed (payload size caps, forwarding templates, etc.)
- Decision informs implementation of card #21 (routing/event fan-out)

## Key Questions from RFC-005

1. Does baggage forwarding align with loose coupling preference?
2. Comfortable with plugin author discipline for manual forwarding?
3. At what chain length would automatic accumulation be needed?
4. Should large payloads (>100KB) be forbidden in events?
5. Any hybrid approach not covered?

## References

- `RFC-005-ROUTING_OPTIONS.md` — Full analysis with comparison matrix
- Card #21 — Sprint 2: Routing (implementation depends on this decision)
