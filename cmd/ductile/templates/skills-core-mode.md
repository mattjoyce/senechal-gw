# Ductile Gateway: AI Operator Guide (Core Mode)

> **No config loaded.** This is the token-frugal baseline skill set available without
> a live configuration. Use it to orient and bootstrap — then load a full manifest.

---

## RFC-004 Alignment

This guide follows the RFC-004 progressive-disclosure design: start with the minimal
viable context, then pull the full capability manifest only when you need to invoke
plugins or pipelines.

---

## Core CLI Loop

Use these commands to interrogate and operate the gateway without loading a config:

```
ductile config check --json
ductile system status --json
ductile job inspect <job_id>
```

- **`ductile config check --json`** — Validate configuration syntax, policy, and
  integrity checksums. Run this first when the gateway behaves unexpectedly.
- **`ductile system status --json`** — Check gateway health: PID lock, state DB,
  config load, and plugin reachability.
- **`ductile job inspect <job_id>`** — Retrieve logs, baggage, and workspace
  artifacts for a specific job execution.

---

## Loading the Full Manifest

Once config is accessible, load the complete skill manifest:

```
ductile system skills --config <config-dir>
```

Or make config auto-discoverable:

```
export DUCTILE_CONFIG_DIR=<config-dir>
ductile system skills
```

The full manifest includes:
- All plugin commands with invocation endpoints, semantic anchors
  (mutates_state, idempotent, retry_safe), and schemas
- All configured pipelines with trigger and execution-mode details

---

**Next step:** Run `ductile system skills --config <config-dir>` to load the live
LLM Operator Skill Manifest for this deployment.
