# Ductile Deployment Guide

This document describes how to deploy a host-local Ductile instance as a
systemd user service. It reflects the first reference deployment on
`matt-ThinkPad-T14s-Gen-1` (2026-02-22) and is the canonical procedure for
repeating this on other hosts.

See also: RFC-006 (local execution plane topology).

---

## 1. Build the Binary

Build from source and install to the user's local bin:

```bash
cd /home/matt/Projects/ductile
go build -o ~/.local/bin/ductile ./cmd/ductile
```

Verify:
```bash
ductile --version
```

The binary is self-contained — no additional runtime dependencies.

---

## 2. Directory Layout

Create a deployment root with separate `config/` and `data/` directories:

```
ductile-local/
├── config/
│   ├── config.yaml      # main config (includes others via `include:`)
│   ├── api.yaml         # API listen address + auth tokens
│   └── plugins.yaml     # plugin enable/config
└── data/
    ├── ductile.db       # SQLite state DB (created on first start)
    └── outputs/         # write target for file_handler plugin
```

Create it:
```bash
mkdir -p ~/admin/ductile-local/config ~/admin/ductile-local/data/outputs
```

---

## 3. Split Config Pattern

Ductile supports modular ("grafted") configs via the `include:` key. The main
config file sets global options and includes the others by relative path.

### config/config.yaml

```yaml
log_level: info

state:
  database:
    path: ./data/ductile.db

plugin_roots:
  - /home/matt/Projects/ductile/plugins

include:
  - api.yaml
  - plugins.yaml
```

`plugin_roots` is a list of directories to scan for plugin executables at
startup. Any plugin binary found here is *discovered*; only plugins listed in
`plugins.yaml` are *configured* (and those not listed emit a warning but still
load).

### config/api.yaml

```yaml
api:
  enabled: true
  listen: "localhost:8081"
  auth:
    tokens:
      - token: <your-token>
        scopes: ["*"]
```

Generate a token:
```bash
openssl rand -hex 32
```

Store the token in your shell environment:
```bash
# ~/.zshrc
export DUCTILE_LOCAL_TOKEN=<your-token>
```

### config/plugins.yaml

```yaml
plugins:
  fabric:
    enabled: true
    timeout: 120s
    max_attempts: 2
    config:
      FABRIC_DEFAULT_PATTERN: "summarize"

  file_handler:
    enabled: true
    timeout: 30s
    max_attempts: 1
    config:
      allowed_read_paths: "/home/matt"
      allowed_write_paths: "/home/matt/admin/ductile-local/data/outputs"

  jina-reader:
    enabled: true
    timeout: 30s
    max_attempts: 3
    circuit_breaker:
      threshold: 3
      reset_after: 5m
    config: {}
```

---

## 4. Validate Config

Before starting the service, validate the configuration:

```bash
cd ~/admin/ductile-local
ductile config check --config config/config.yaml
```

Expected output:
```
Configuration valid (N warning(s))
  WARN  [unused] plugin "echo" discovered but not referenced in config
  ...
```

Warnings about undeclared plugins are expected if `plugin_roots` contains
plugins you haven't explicitly configured. They are loaded but not usable
without config entries.

---

## 5. systemd User Service

Create `~/.config/systemd/user/ductile-local.service`:

```ini
[Unit]
Description=Ductile Gateway (local prod)
After=network.target

[Service]
Type=simple
WorkingDirectory=/home/matt/admin/ductile-local
ExecStart=/home/matt/.local/bin/ductile system start --config config/config.yaml
Restart=on-failure
RestartSec=5s
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=default.target
```

Enable and start:
```bash
systemctl --user daemon-reload
systemctl --user enable --now ductile-local
```

Check status:
```bash
systemctl --user status ductile-local
journalctl --user -u ductile-local -f
```

---

## 6. Verification Checklist

After starting the service, verify the following:

```bash
# Health — no auth required
curl http://localhost:8081/healthz

# Expected:
# {"status":"ok","uptime_seconds":N,"queue_depth":0,"plugins_loaded":5,"plugins_circuit_open":0}

# Plugin list — requires auth
curl -H "Authorization: Bearer $DUCTILE_LOCAL_TOKEN" http://localhost:8081/plugins

# OpenAPI schema — no auth required
curl http://localhost:8081/openapi.json | head -20
```

Confirm:
- [ ] `status: ok` in healthz
- [ ] `plugins_loaded` > 0
- [ ] fabric, file_handler, jina-reader appear in `/plugins`
- [ ] `/openapi.json` returns valid JSON

---

## 7. RFC-006 Topology Notes

RFC-006 defines two Ductile instance roles:

| Role | Purpose |
|---|---|
| **Boundary node** | Public-facing gateway, handles external API calls, auth, routing |
| **Host-local node** | Per-host execution plane, runs plugins with local resource access |

This deployment is a **host-local node**:
- Listens on `localhost` only (not exposed to LAN)
- Token scoped to `["*"]` for local use
- Plugins have access to local filesystem (`file_handler`) and local tools (`fabric`)
- Receives work dispatched from a boundary node or local AgenticLoop agent

The prod Unraid instance (`192.168.20.4:8888`) is the boundary node for this network.

---

## 8. Updating the Binary

When a new version is built:

```bash
# Stop the service first (optional but clean)
systemctl --user stop ductile-local

# Rebuild
cd /home/matt/Projects/ductile
go build -o ~/.local/bin/ductile ./cmd/ductile

# Restart
systemctl --user start ductile-local
systemctl --user status ductile-local
```

Or just rebuild and restart in one shot — the service will pick up the new
binary on next start:
```bash
cd /home/matt/Projects/ductile && go build -o ~/.local/bin/ductile ./cmd/ductile && systemctl --user restart ductile-local
```
