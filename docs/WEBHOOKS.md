# Webhooks

This document summarizes how webhooks work in Ductile and how to configure them safely.

## Overview

- Webhooks run on a dedicated HTTP listener and enqueue plugin jobs.
- Every webhook requires **HMAC-SHA256** signature verification.
- Webhooks are configured in **`webhooks.yaml`** (a high-security file).
- The webhook listener is configured via `webhooks.listen` in `config.yaml`.

## Configuration Model

Ductile supports two config modes:

### 1) Directory mode

If the config directory contains `config.yaml` plus a `plugins/` or `pipelines/` directory, Ductile operates in **directory mode** and automatically loads `webhooks.yaml` from the config root.

### 2) Include mode (most common locally)

If `config.yaml` has an `include:` list, **only** files listed there are merged. In this mode you must explicitly include `webhooks.yaml`:

```yaml
include:
  - api.yaml
  - plugins.yaml
  - webhooks.yaml
```

If `webhooks.yaml` is not included, the listener will start **without endpoints**, and requests will return 404.

## File Layout

Typical config root:

```
~/.config/ductile/
├── config.yaml
├── api.yaml
├── plugins.yaml
├── webhooks.yaml
└── .checksums
```

## webhooks.yaml Format

```yaml
webhooks:
  endpoints:
    - name: astro_rebuild_staging
      path: /webhook/astro-rebuild-staging
      plugin: astro_rebuild_staging
      secret_ref: astro_webhook_secret
      signature_header: X-Ductile-Signature-256
      max_body_size: 1MB                 # optional
```

Notes:
- `secret_ref` is required and must reference tokens.yaml.
- `signature_header` is mandatory.
- `max_body_size` defaults to 1MB.

## Listener Port

Set in `config.yaml`:

```yaml
webhooks:
  listen: "127.0.0.1:8091"
```

## Triggering a Webhook

Example HMAC signature (GitHub-style `sha256=` prefix is supported):

```bash
payload='{"payload":{"reason":"manual rebuild"}}'
sig=$(printf '%s' "$payload" | \
  openssl dgst -sha256 -hmac '<secret>' -hex | awk '{print $2}')

curl -sS -X POST http://127.0.0.1:8091/webhook/astro-rebuild-staging \
  -H "Content-Type: application/json" \
  -H "X-Ductile-Signature-256: sha256=$sig" \
  -d "$payload"
```

Response returns a `job_id` when accepted.

## Security & Integrity

- `webhooks.yaml` is **high security** and must be sealed with `ductile config lock`.
- In `strict_mode: true`, tampering will prevent startup.
- Use `secret_ref` + `tokens.yaml` for secret management in production.

## Operational Checks

- Webhook listener health: `GET /healthz` on the webhook listener port.
- Verify endpoints are loaded:
  ```bash
  ductile config show --config-dir ~/.config/ductile | rg -n "webhooks" -C 3
  ```
