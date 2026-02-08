---
id: 6
status: done
priority: Normal
blocked_by: []
tags: [security]
---

# Security Model

Auth, secrets management, and sandboxing for connectors.

## Acceptance Criteria
- Secrets storage approach is defined (encrypted file, env vars, vault)
- Connector permission boundaries are designed
- OAuth flow handling for external services is addressed
- API authentication for inbound requests is specified
- Logging sanitization rules are documented

## Narrative
- 2026-02-08: Created during initial project discussion. Primer article highlights access control, data exposure in logs, and malicious input as key vulnerabilities. (by @assistant)
- 2026-02-08: Addressed in RFC-002 Decisions. Secrets: env var interpolation in config, credentials in static config, tokens in dynamic plugin state. OAuth: plugin-owned, not core. Webhooks: mandatory HMAC-SHA256, configurable signature header, 403 on failure with no details. Plugin sandboxing: must live under plugins_dir, symlinks resolved, path traversal rejected, world-writable dirs refused, systemd User= recommended. Logging: no redaction in V1 — operator responsibility, don't log secrets. (by @assistant)
