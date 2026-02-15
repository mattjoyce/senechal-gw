# Ductile (ductile)

[![Go Version](https://img.shields.io/badge/go-1.25.4-blue.svg)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**Ductile is an LLM Boundary Layer Affordance.**

It sits between model intent and real-world side effects, turning "do this" into safe, bounded, and observable execution.

In practical terms: Ductile is the control plane that gives an LLM useful affordances for operations without giving it unconstrained access to your systems.

---

## What This Means

An LLM is good at intent and synthesis, but weak at execution safety.

Ductile provides the missing layer:

- **Bounded capability surface:** plugin manifests, command types, and scoped auth tokens.
- **Safe execution semantics:** queued jobs, retries, backoff, poll guards, and circuit breakers.
- **Governed routing:** explicit pipeline/event transitions instead of ad-hoc tool chaining.
- **Operator controls:** dry-run paths, config integrity checks, and reset/diagnostic utilities.
- **Auditability:** event context lineage, structured logs, job history, and real-time event streams.

## What Ductile Is (and Is Not)

- **Is:** a governed execution boundary for LLM and human operators.
- **Is:** a reliability-first integration runtime with clear operational controls.
- **Is not:** a general autonomous agent framework.
- **Is not:** a direct "LLM-to-external-system" bypass.

## Architecture (Mental Model)

```text
LLM/Human Intent
      |
      v
 Ductile Boundary Layer
  - Auth + scopes
  - Policy + routing
  - Queue + retries + circuit breakers
  - Event context + audit trail
      |
      v
Polyglot Plugins -> External APIs/Systems
```

## Documentation Index

We have reorganized our documentation into role-based manuals to improve clarity for both humans and LLM operators.

### [Getting Started](docs/GETTING_STARTED.md)
The absolute first stop. Installation, basic usage, and core CLI principles.

### [Operator Guide](docs/OPERATOR_GUIDE.md)
The manual for running the system. Monitoring (TUI), system maintenance, and API control.

### [Plugin Development](docs/PLUGIN_DEVELOPMENT.md)
A guide for developers building new capabilities (Skills) using Protocol v2.

### [Architecture & Pipelines](docs/ARCHITECTURE.md)
Deep dive into the engine, the Governance Hybrid model, and the Pipeline DSL.

### [Configuration Reference](docs/CONFIG_REFERENCE.md)
Strict technical specification for YAML structure and integrity verification.

---

## Key Features

- **Polyglot Plugins:** Write logic in any language. If it can read `stdin` and write `stdout`, it's a plugin.
- **Reliability First:** Crash recovery, retries with exponential backoff, and circuit breakers.
- **Governance Hybrid:** Automatic metadata ("Baggage") accumulation and zero-copy workspace cloning.
- **Security by Design:** BLAKE3-based integrity verification and scoped Bearer tokens.
- **Real-Time Monitoring:** SSE event stream and a built-in "btop-style" TUI dashboard.

## Quick Start

```bash
# Build
go build -o ductile ./cmd/ductile

# Start the gateway
./ductile system start

# Launch the monitor/watch UI (in another terminal)
./ductile system monitor --api-key "your-token"
```

## License
MIT. See [LICENSE](LICENSE) for details. (Note: Project is in MVP stage).
