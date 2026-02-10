---
id: 42
status: todo
priority: High
blocked_by: [39]
tags: [sprint-3, webhooks, security]
---

# Webhook Listener with HMAC Verification

Implement HTTP webhook endpoints with HMAC-SHA256 signature verification for secure 3rd party integrations.

## Acceptance Criteria

- POST /webhook/{path} endpoints configured in webhooks.yaml
- HMAC-SHA256 signature verification (mandatory)
- 403 on invalid signature with minimal error details
- Body size limit enforced (default 1MB, configurable)
- Enqueues job for configured plugin:command
- Request body passed to plugin as event payload
- Proper error handling and logging

## Implementation Details

**Package:** `internal/webhook/`

**Configuration (webhooks.yaml):**
```yaml
webhooks:
  github-push:
    path: /webhook/github
    hmac_secret: ${GITHUB_WEBHOOK_SECRET}
    body_size_limit: 1048576  # 1MB
    plugin: github
    command: handle
```

**HMAC Verification:**
- Extract signature from header (e.g., `X-Hub-Signature-256`)
- Compute HMAC-SHA256 of request body using configured secret
- Constant-time comparison to prevent timing attacks
- 403 with generic error on mismatch (no details leaked)

**Job Enqueueing:**
- Extract webhook path from URL
- Look up configuration by path
- Create job with plugin, command from config
- Pass request body as event payload
- Return 202 Accepted with job_id

**Security:**
- HMAC verification is mandatory (no plaintext webhooks)
- Body size limits prevent DoS
- No signature details in error responses
- Request logging excludes sensitive payloads

## Testing

- Valid HMAC → job enqueued, 202 returned
- Invalid HMAC → 403, no job created
- Missing signature header → 403
- Body exceeds limit → 413 Payload Too Large
- Unknown webhook path → 404
- Integration test with real GitHub webhook payload

## Dependencies

- Multi-file config (#39) - webhooks.yaml loading
- Job queue (#4, #5) - job enqueueing
- Existing HTTP server from Sprint 2

## Narrative

