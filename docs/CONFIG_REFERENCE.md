# Ductile: Configuration Specification

**Version:** 1.1 (Tiered Directory Model)  
**Date:** 2026-02-12  
**Status:** Approved  

This document defines the configuration structure, integrity verification, and runtime compilation behavior for Ductile.

---

## 1. Directory Structure

Ductile uses a "Nagios-style" configuration directory, typically located at `~/.config/ductile/`.

```
~/.config/ductile/
├── config.yaml                  # [Operational] Service-level settings
├── webhooks.yaml                # [High Security] Webhook endpoints & secrets
├── tokens.yaml                  # [High Security] API token registry
├── routes.yaml                  # [Operational] Global routing rules
├── plugins/                     # [Operational] Modular plugin configs
│   ├── echo.yaml
│   └── withings.yaml
├── pipelines/                   # [Operational] Modular pipeline definitions
│   └── wisdom.yaml
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
| **Operational** | `config.yaml`, `plugins/*.yaml`, `pipelines/*.yaml`, `routes.yaml` | **Warn & Continue**: Logs a warning but loads the file. |

### 2.1 The Seal (`.checksums`)
The `.checksums` file is a YAML manifest containing BLAKE3 hashes indexed by the **absolute path** of every authorized file.
- **System Lock-in**: Moving the configuration directory breaks the seal.
- **Authorization**: The `ductile config lock` command is the only way to update the manifest.

---

## 3. Monolithic Compilation (Grafting)

At runtime, the gateway compiles all discovered files into a single, monolithic configuration object.

### 3.1 Merge Logic
- **Root First**: `config.yaml` is loaded first as the base.
- **Sequential Grafting**: Modular files from `plugins/` and `pipelines/` are grafted onto the monolith in alphabetical order.
- **Precedence**: Later entries override earlier ones (n-1 branching).
- **Matching Branches**:
    - **Maps (e.g., `plugins:`)**: Keys are merged. Duplicate keys are overridden by the later file.
    - **Arrays (e.g., `pipelines:`, `routes:`)**: Items are **appended** to the list.
    - **Scalars**: Later values replace earlier ones.

### 3.2 Modular Example
**config.yaml (Root)**
```yaml
service:
  name: my-gateway
```

**pipelines/wisdom.yaml**
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

---

## 4. File Formats

### 4.1 config.yaml (Service settings)
```yaml
service:
  name: ductile
  plugins_dir: /opt/ductile/plugins
  tick_interval: 60s
  log_level: info
  log_format: json
  dedupe_ttl: 24h
  job_log_retention: 30d

api:
  enabled: true
  listen: 127.0.0.1:8080

state:
  path: ./data/state.db
```

### 4.2 plugins/*.yaml (Plugin definitions)
```yaml
plugins:
  echo:
    enabled: true
    schedule:
      every: 5m
    config:
      message: "Hello"
```

### 4.3 webhooks.yaml (High Security)
```yaml
webhooks:
  - name: github
    path: /webhook/github
    plugin: github-handler
    secret: ${GITHUB_WEBHOOK_SECRET}
    signature_header: X-Hub-Signature-256
```

### 4.4 tokens.yaml (High Security)
```yaml
tokens:
  - name: admin-cli
    key: ${ADMIN_API_KEY}
    scopes_file: scopes/admin-cli.json
    scopes_hash: blake3:a3f8c2d9...
```

---

## 5. Authentication Configuration

Ductile supports two authentication modes. These are configured within the `api` section of the configuration (typically in `config.yaml` or a dedicated `auth.yaml`).

### 5.1 Legacy: Single API Key (Simple)
For single-user or development environments.
```yaml
api:
  auth:
    api_key: your_secret_token
```
**Access Level**: Full admin (`*` scope).
**Note**: This field is intended for simple setups and is planned for deprecation in future versions.

### 5.2 Modern: Scoped Tokens (Recommended)
For multi-user or production environments.
```yaml
api:
  auth:
    tokens:
      - token: admin_token
        scopes: ["*"]
      - token: readonly_token
        scopes: ["read:*"]
      - token: github_token
        scopes: ["github-handler:rw", "read:jobs"]
```

### 5.3 Coexistence and Migration
- If both `api_key` and `tokens` are provided, both remain valid.
- **Migration Path**:
  1. Add clients to the `tokens` array.
  2. Verify client connectivity.
  3. Remove the `api_key` field.

### 5.4 Token Scopes
Scopes can be manifest-driven or granular:
- `*`: Full admin access.
- `read:*`: Access to all GET endpoints.
- `{plugin}:ro`: Read-only access to a plugin (mapped to `read` commands in manifest).
- `{plugin}:rw`: Read-write access to a plugin (mapped to `read` and `write` commands).
- `trigger:{plugin}:{command}`: Specific permission to trigger a command.

---

## 6. Environment Interpolation

Interpolation of `${VAR}` syntax happens **after** integrity verification but **before** YAML parsing.
- Secrets must never be stored in YAML files; use environment variables.
- Interpolation is **forbidden** in file paths (e.g., `include:` or directory walking) to ensure a static, verifiable tree.
