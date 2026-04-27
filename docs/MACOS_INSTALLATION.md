---
audience: [3]
form: tutorial
density: learner
verified: 2026-04-27
---

# Ductile on macOS — Installation Guide

This guide documents the first installation of Ductile on macOS (darwin/arm64, macOS 15 Sequoia). It covers the differences from the Linux/systemd deployment documented in [DEPLOYMENT.md](DEPLOYMENT.md).

---

## Platform Differences at a Glance

| Concern | Linux | macOS |
|---|---|---|
| **Service manager** | systemd | launchd |
| **Service unit file** | `~/.config/systemd/user/*.service` | `~/Library/LaunchAgents/*.plist` |
| **Enable/start** | `systemctl --user enable --now` | `launchctl bootstrap gui/$(id -u)` |
| **Status** | `systemctl --user status` | `launchctl list \| grep ductile` |
| **Logs** | `journalctl --user -u ductile-local` | `tail -f ~/Library/Logs/ductile-local.log` |
| **User bin dir** | `~/.local/bin/` (in PATH by default) | `~/.local/bin/` (add to PATH if needed) |
| **Restart policy** | `Restart=on-failure` | `KeepAlive=true` + `ThrottleInterval` |

---

## 1. Prerequisites

- macOS 13 Ventura or later (tested on macOS 15 Sequoia, arm64)
- Go ≥ 1.24.3 — install via Homebrew: `brew install go`
- Git

Verify Go:
```bash
go version
# go version go1.26.0 darwin/arm64
```

---

## 2. Clone and Build

```bash
git clone git@github.com:mattjoyce/ductile.git ~/Projects/ductile
cd ~/Projects/ductile
go build -ldflags "$(./scripts/version.sh)" -o ductile ./cmd/ductile
```

> **Note:** On macOS, `/usr/local/bin/` requires `sudo` to write to. Install to `~/.local/bin/` instead (create it if it doesn't exist and ensure it's in `$PATH`):

```bash
mkdir -p ~/.local/bin
cp ductile ~/.local/bin/ductile

# Add to PATH if not already present — add this line to ~/.zshrc:
export PATH="$HOME/.local/bin:$PATH"
```

Verify:
```bash
ductile --version
# ductile <version>
# commit: <hash>
# built_at: <timestamp>
```

---

## 3. Config Directory

Ductile uses `~/.config/ductile/` by default (XDG-style, same as Linux).

Create the directory and a minimal working config:

```bash
mkdir -p ~/.config/ductile
```

### config.yaml

```yaml
log_level: info

service:
  strict_mode: false

state:
  # Use an absolute path — tilde expansion in this field resolves relative
  # to the config directory, not $HOME, on some ductile versions.
  path: "/Users/YOUR_USERNAME/.config/ductile/ductile.db"

plugin_roots:
  # Absolute path to the built-in plugins in the cloned source repo.
  # Tilde is NOT expanded here — use full paths.
  - "/Users/YOUR_USERNAME/Projects/ductile/plugins"

include:
  - api.yaml
  - plugins.yaml
  - pipelines.yaml
  - webhooks.yaml
```

> **macOS gotcha:** Unlike the Linux deployment, `~` in `plugin_roots` and `state.path`
> is resolved relative to the config directory, not `$HOME`. Use absolute paths for both.

### api.yaml

Generate a token first:
```bash
openssl rand -hex 32
```

```yaml
api:
  enabled: true
  listen: "127.0.0.1:8082"   # Use 8082 if 8081 is taken by another ductile instance
  auth:
    tokens:
      - token: "PASTE_YOUR_TOKEN_HERE"
        scopes: ["*"]
```

Store the token in your shell environment:
```bash
# ~/.zshrc
export DUCTILE_LOCAL_TOKEN=<your-token>
```

### plugins.yaml

Start with the built-in `echo` plugin to verify the setup:

```yaml
plugins:
  echo:
    enabled: true
    schedules:
      - id: default
        every: 5m
        jitter: 30s
    config:
      message: "Hello from Ductile on Mac!"
```

### pipelines.yaml

```yaml
pipelines: []
```

### webhooks.yaml

```yaml
webhooks:
  listen: "127.0.0.1:8091"
  endpoints: []
```

---

## 4. Lock the Config

Ductile verifies config integrity via checksums. After writing all config files, lock them:

```bash
ductile config lock --config ~/.config/ductile/
# Successfully locked configuration in 1 directory/ies:
#   - /Users/YOUR_USERNAME/.config/ductile
```

Re-run this after any config change. Ductile will refuse to start if the checksums don't match.

---

## 5. Foreground Test

Verify the setup runs cleanly before installing as a service:

```bash
ductile system start --config ~/.config/ductile/
```

In another terminal:
```bash
curl http://127.0.0.1:8082/healthz
# {"status":"ok","uptime_seconds":N,"queue_depth":0,"plugins_loaded":10,...}
```

Press `Ctrl+C` to stop.

---

## 6. launchd Service

macOS uses **launchd** instead of systemd. Create a `LaunchAgent` plist for user-session auto-start:

```bash
mkdir -p ~/Library/LaunchAgents
```

Create `~/Library/LaunchAgents/com.mattjoyce.ductile-local.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.mattjoyce.ductile-local</string>

    <key>ProgramArguments</key>
    <array>
        <string>/Users/YOUR_USERNAME/.local/bin/ductile</string>
        <string>system</string>
        <string>start</string>
        <string>--config</string>
        <string>/Users/YOUR_USERNAME/.config/ductile/</string>
    </array>

    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin</string>
        <key>HOME</key>
        <string>/Users/YOUR_USERNAME</string>
    </dict>

    <key>RunAtLoad</key>
    <true/>

    <key>KeepAlive</key>
    <true/>

    <key>StandardOutPath</key>
    <string>/Users/YOUR_USERNAME/Library/Logs/ductile-local.log</string>

    <key>StandardErrorPath</key>
    <string>/Users/YOUR_USERNAME/Library/Logs/ductile-local.log</string>

    <key>ThrottleInterval</key>
    <integer>5</integer>
</dict>
</plist>
```

Replace `YOUR_USERNAME` with your macOS username (e.g. `mattjoyce`). Absolute paths are required — launchd does not expand `~`.

> **Why `ThrottleInterval: 5`?** Combined with `KeepAlive`, this prevents tight restart loops if ductile crashes on startup (e.g. config validation failure). It mirrors `RestartSec=5s` in systemd.

---

## 7. launchctl Commands

```bash
# Load and start (survives reboots)
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.mattjoyce.ductile-local.plist

# Check if running (PID in first column means running, 0/-1 means stopped/failed)
launchctl list | grep ductile

# Stop
launchctl stop com.mattjoyce.ductile-local

# Start (if already loaded)
launchctl start com.mattjoyce.ductile-local

# Unload (remove from launchd entirely)
launchctl bootout gui/$(id -u) ~/Library/LaunchAgents/com.mattjoyce.ductile-local.plist

# View logs
tail -f ~/Library/Logs/ductile-local.log
```

> **launchd vs systemd vocabulary:**
> - `bootstrap` = `systemctl enable --now` (load and start, persist across reboots)
> - `bootout` = `systemctl disable --now` (unload and stop, remove persistence)
> - `start/stop` = `systemctl start/stop` (one-shot, already-loaded service)
> - `launchctl list` = `systemctl status` (check running state)

---

## 8. Verification Checklist

After the launchd service is running:

```bash
# Health — no auth required
curl http://127.0.0.1:8082/healthz
# {"status":"ok","uptime_seconds":N,"queue_depth":0,"plugins_loaded":N,...}

# Plugin list — requires auth
curl -H "Authorization: Bearer $DUCTILE_LOCAL_TOKEN" http://127.0.0.1:8082/plugins

# Logs
tail -20 ~/Library/Logs/ductile-local.log
```

Confirm:
- [ ] `status: ok` in healthz
- [ ] `plugins_loaded` > 0
- [ ] echo plugin appears in `/plugins`
- [ ] Log file exists at `~/Library/Logs/ductile-local.log`

---

## 9. Updating the Binary

```bash
cd ~/Projects/ductile
git pull

go build -ldflags "$(./scripts/version.sh)" -o ~/.local/bin/ductile ./cmd/ductile

# Restart the service
launchctl stop com.mattjoyce.ductile-local
launchctl start com.mattjoyce.ductile-local
```

After updating config files, always re-lock before restarting:
```bash
ductile config lock --config ~/.config/ductile/
launchctl stop com.mattjoyce.ductile-local
launchctl start com.mattjoyce.ductile-local
```

---

## Known Differences from Linux Deployment

1. **No `~` expansion in config YAML** — `plugin_roots` and `state.path` do not expand `~`. Use absolute paths (e.g. `/Users/mattjoyce/...`).
2. **No EnvironmentFile equivalent** — launchd plist `EnvironmentVariables` replaces systemd's `EnvironmentFile`. Secrets must be inlined or loaded by the process at runtime.
3. **launchd owns PATH** — plugins that shell out (e.g. `sys_exec`) inherit only the PATH set in the plist, not your shell's PATH. Add Homebrew (`/opt/homebrew/bin`) explicitly.
4. **`strict_mode: false` recommended initially** — On first install, strict mode will reject config files with warnings. Disable until the config is stable, then re-enable.

---

## See Also

- [DEPLOYMENT.md](DEPLOYMENT.md) — Linux/systemd reference deployment
- [GETTING_STARTED.md](GETTING_STARTED.md) — Quickstart with the echo plugin
- [CONFIG_REFERENCE.md](CONFIG_REFERENCE.md) — Full config schema reference
- [OPERATOR_GUIDE.md](OPERATOR_GUIDE.md) — Day-2 operations, monitoring, maintenance
