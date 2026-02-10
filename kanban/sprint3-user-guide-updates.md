---
id: 44
status: todo
priority: Normal
blocked_by: []
assignee: "@gemini"
tags: [sprint-3, documentation]
---

# Update User Guide for Sprint 3 Features

Update `docs/USER_GUIDE.md` to document the new Sprint 3 features: multi-file config, webhooks, token scopes, and observability endpoints.

## Scope

Add documentation for Sprint 3 features shipped in PRs #11 and #12:

1. **Multi-file configuration** (#39)
2. **Webhook endpoints with HMAC** (#42)
3. **Manifest command types** (#36)
4. **Token scopes and authorization** (#35)
5. **SSE /events endpoint** (#33)
6. **/healthz endpoint** (#43)

## Sections to Update

### 1. Configuration (New Section)

Add section after "Getting Started" covering:
- Multi-file config directory structure (`~/.config/senechal-gw/`)
- Include array in config.yaml
- Environment variable interpolation in includes
- BLAKE3 hash verification for sensitive files
- Example multi-file setup

**Example structure:**
```yaml
# config.yaml
service:
  plugins_dir: ./plugins
include:
  - plugins.yaml
  - webhooks.yaml
  - ${ENV}/tokens.yaml  # Environment-specific
```

### 2. Webhooks (New Section)

Add section covering:
- Webhook configuration in webhooks.yaml
- HMAC-SHA256 signature verification
- Setting up GitHub webhooks (example)
- Testing webhooks locally
- Security best practices

**Example:**
```yaml
# webhooks.yaml
webhooks:
  github-push:
    path: /webhook/github
    hmac_secret: ${GITHUB_WEBHOOK_SECRET}
    plugin: github
    command: handle
```

### 3. Authentication & Authorization (New Section)

Add section covering:
- Token-based API authentication
- Scope definitions and expansion
- Manifest command types (read vs write)
- Creating scoped tokens
- Example: read-only token for monitoring

**Example:**
```yaml
# tokens.yaml
tokens:
  - name: monitoring
    scopes_file: scopes/monitoring.json
    scopes_hash: blake3:abc123...
```

### 4. Monitoring & Observability (New Section)

Add section covering:
- GET /healthz endpoint (status, uptime, queue depth)
- GET /events SSE endpoint (real-time debugging)
- Example: Monitoring with curl
- Example: Streaming events in browser

**Example:**
```bash
# Check health
curl http://localhost:8080/healthz

# Stream events
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/events
```

### 5. Update Existing Sections

**API Endpoints section:**
- Add webhook endpoints documentation
- Add /events and /healthz to API reference
- Update authentication examples

**Plugin Development section:**
- Document command type metadata in manifest
- Show both array and object command formats
- Explain read vs write command types

## Style Guidelines

- Match existing USER_GUIDE.md tone and structure
- Use concrete examples (config snippets, curl commands)
- Include "Why this matters" context for each feature
- Add troubleshooting tips where relevant
- Keep explanations concise (2-3 paragraphs max per feature)
- Use code blocks with syntax highlighting

## Acceptance Criteria

- ✅ All 6 Sprint 3 features documented
- ✅ Code examples tested and working
- ✅ New sections fit naturally into existing guide structure
- ✅ No spelling/grammar errors
- ✅ Markdown properly formatted
- ✅ Links to relevant SPEC.md sections where appropriate
- ✅ Document is 3500-4500 words (added ~1500 words to existing guide)

## Testing

**Before submitting PR:**
1. Read through entire guide start-to-finish
2. Verify all code examples are copy-pasteable
3. Test curl commands against running service
4. Check markdown renders correctly (use preview)
5. Spell check

## Deliverable

Updated `docs/USER_GUIDE.md` with comprehensive Sprint 3 feature documentation.

## Branch

`gemini/sprint3-user-guide`

## Estimated Effort

2-3 hours (research features + write + examples + review)

## References

- Existing `docs/USER_GUIDE.md`
- PR #11 implementation (multi-file config, webhooks)
- PR #12 implementation (command types, scopes, events, healthz)
- `SPEC.md` §6 (Protocol), §8.2 (Webhooks), §11 (Configuration)
- Sprint 3 kanban cards (#39, #42, #36, #35, #33, #43) for context

## Narrative

