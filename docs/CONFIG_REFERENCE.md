# Ductile: Configuration Specification

**Version:** 1.1 (Tiered Directory Model)  
**Date:** 2026-02-25  
**Status:** Approved  

This document defines the configuration structure, integrity verification, and runtime compilation behavior for Ductile.

---

## 1. Directory Structure

Ductile uses a configuration directory, typically located at `~/.config/ductile/`. Only
`config.yaml` is implicitly loaded; all other files must be referenced via `include:`.

```
~/.config/ductile/
├── config.yaml                  # [Operational] Service-level settings
├── webhooks.yaml                # [High Security] Webhook endpoints & secrets (include explicitly)
├── tokens.yaml                  # [High Security] API token registry (include explicitly)
├── routes.yaml                  # [Operational] Global routing rules (include explicitly)
└── scopes/                      # [High Security] Token scope definitions
    ├── admin-cli.json
    └── github-integration.json
```

---

## 2. Tiered Integrity Preflight

Before starting, the system verifies all files against a monolithic `.checksums` manifest located in the configuration root. Integrity is enforced in two tiers:

| Tier | Files | Missing/Mismatch Behavior |
| :--- | :--- | :--- |
| **High Security** | `tokens.yaml`, `webhooks.yaml`, `scopes/*.json` | **Hard Fail**: System refuses to start (EX_CONFIG). |
| **Operational** | `config.yaml`, `routes.yaml` | **Warn & Continue**: Logs a warning but loads the file (Unless `strict_mode: true` is set, in which case it is a **Hard Fail**). |

### 2.1 The Seal (`.checksums`)
The `.checksums` file is a YAML manifest containing BLAKE3 hashes indexed by the **absolute path** of every authorized file.
- **System Lock-in**: Moving the configuration directory breaks the seal.
- **Authorization**: The `ductile config lock` command is the only way to update the manifest.

---

## 3. Monolithic Compilation (Grafting)

At runtime, the gateway compiles all discovered files into a single, monolithic configuration object.

### 3.1 Merge Logic
- **Root First**: `config.yaml` is loaded first as the base.
- **Explicit Includes**: Additional files are loaded from the `include:` list (and any directories listed there) in order.
- **Precedence**: Later entries override earlier ones (n-1 branching).
- **Matching Branches**:
    - **Maps (e.g., `plugins:`)**: Keys are merged. Duplicate keys are overridden by the later file.
    - **Arrays (e.g., `pipelines:`, `routes:`)**: Items are **appended** to the list.
    - **Scalars**: Later values replace earlier ones.

### 3.2 Modular Example
**config.yaml (Root)**
```yaml
include:
  - pipelines.yaml

service:
  name: my-gateway
```

**pipelines.yaml**
```yaml
pipelines:
  - name: video-wisdom
    on: discord.link
```

**Resulting Monolith:**
```yaml
service:
  name: my-gateway
pipelines:
  - name: video-wisdom
```

### 3.3 Directory includes

`include:` entries may point at directories. Ductile loads `*.yaml` files
from that directory (non-recursive) in alphabetical order and merges them
as if they were listed explicitly.

---

## 4. File Formats

### 4.1 config.yaml (Service settings)
```yaml
service:
  name: ductile
  tick_interval: 60s
  log_level: info
  log_format: json
  dedupe_ttl: 24h
  job_log_retention: 30d
  max_workers: 1
  strict_mode: true  # Enforce integrity & configuration checks on startup

plugin_roots:
  - /opt/ductile/plugins
  - /opt/ductile/plugins-private

api:
  enabled: true
  listen: 127.0.0.1:8080

state:
  path: ./data/state.db
```

Relative paths (like `./data/state.db`) are resolved against the directory containing `config.yaml`.

`plugin_roots` is the multi-root setting.

Discovery behavior:
- Duplicate roots are ignored after first occurrence.
- Roots are scanned in order; if duplicate plugin names exist across roots, the first discovered plugin is kept and later duplicates are ignored.

### 4.2 Plugin definitions (included file)
```yaml
plugins:
  echo:
    enabled: true
    parallelism: 1
    schedules:   # Optional; omit for event-driven plugins
      - id: default
        every: 5m
    config:
      message: "Hello"
```

### 4.2.1 Concurrency controls

- `service.max_workers`: Global worker cap across all plugins.
- `plugins.<name>.parallelism`: Per-plugin concurrency cap.
- Constraint: `1 <= parallelism <= max_workers`.

Manifest interaction:
- Plugins may declare `concurrency_safe: false` in `manifest.yaml`.
- Such plugins run serial by default (effective parallelism = 1).
- Operators can explicitly override with `plugins.<name>.parallelism > 1`.

### 4.3 webhooks.yaml (High Security - Experimental)

> [!IMPORTANT]  
> Webhook support is currently in early development and may not be fully functional in the current MVP.

```yaml
webhooks:
  - name: github
    path: /webhook/github
    plugin: github-handler
    secret_ref: github_webhook_secret
    signature_header: X-Hub-Signature-256
```

See [WEBHOOKS.md](WEBHOOKS.md) for full configuration details, include-mode caveats, and signing examples.

---

## 4.4 tokens.yaml (High Security)
```yaml
tokens:
  - name: admin-cli
    key: ${ADMIN_API_KEY}
    scopes_file: scopes/admin-cli.json
    scopes_hash: blake3:a3f8c2d9...
```

---

## 4.5 routes.yaml (Operational - Experimental)

> [!IMPORTANT]  
> Global routing rules via `routes.yaml` are experimental. Most users should prefer the `pipelines` DSL for orchestration.

```yaml
routes:
  - from: source-plugin
    event_type: event.name
    to: target-plugin
```

---

## 5. Authentication Configuration

Ductile authentication is configured within the `api` section of the configuration (typically in `config.yaml` or a dedicated `auth.yaml`).

### 5.1 Scoped Tokens
For multi-user or production environments.
```yaml
api:
  auth:
    tokens:
      - token: admin_token
        scopes: ["*"]
      - token: readonly_token
        scopes: ["plugin:ro", "jobs:ro", "events:ro"]
      - token: operator_token
        scopes: ["plugin:rw", "jobs:rw", "events:ro"]
```

### 5.2 Token Scopes
Scopes are explicit:
- `*`: Full admin access.
- `plugin:ro`, `plugin:rw`: Plugin and pipeline trigger access.
- `jobs:ro`, `jobs:rw`: Job read/write access.
- `events:ro`, `events:rw`: Event stream access.

---

## 6. Environment Interpolation

Interpolation of `${VAR}` syntax happens **after** integrity verification but **before** YAML parsing.
- Secrets must never be stored in YAML files; use environment variables.
- Interpolation is **forbidden** in file paths (e.g., `include:` or directory walking) to ensure a static, verifiable tree.

### 6.1 Environment file includes

You can preload env vars from `.env` files before interpolation:

```yaml
environment_vars:
  include:
    - .env
```

Notes:
- Paths are resolved relative to the file declaring the include.
- Existing process environment variables are not overridden.
