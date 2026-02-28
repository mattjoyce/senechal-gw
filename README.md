# Ductile

[![Go Version](https://img.shields.io/badge/go-1.25.4-blue.svg)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**Lightweight integration engine for small-scale automation.**

Full integration engines (n8n, Huginn, Node-RED) are too heavy for personal or homelab use. Cron jobs and personal APIs lack the features you actually want — event routing, pipelines, webhooks, retry logic. Ductile fills that gap.

A single Go binary orchestrates polyglot plugins via a simple JSON protocol. Write connectors in any language. Configure pipelines in YAML. No cluster, no managed services, no ops overhead.

---

## Features

- **Polyglot plugins** — any language, any script. If it reads `stdin` and writes `stdout`, it works.
- **Event routing & pipelines** — chain connectors, fan out events, build multi-hop workflows.
- **Scheduler** — fuzzy intervals, jitter, circuit breakers for unattended operation.
- **Webhooks** — inbound HMAC-verified endpoints.
- **Reliable by default** — SQLite-backed queue, crash recovery, at-least-once delivery.
- **LLM-operable** — self-describing via `--skills`, API-driven for agentic workflows.

---

## Quick Start

```bash
# Build
go build -o ductile ./cmd/ductile

# Start the gateway
./ductile system start

# Launch the monitor TUI (in another terminal)
./ductile system watch
```

![Ductile system watch TUI](docs/Ductile-system-watch-screenshot.png)

## Documentation Index

- [**Getting Started**](docs/GETTING_STARTED.md) — Installation and basic usage.
- [**The Glossary**](docs/GLOSSARY.md) — Key terms and concepts.
- [**Cookbook**](docs/COOKBOOK.md) — Common patterns (Discord, YouTube, Astro).
- [**Core Mechanics**](docs/ARCHITECTURE.md) — Architecture and design decisions.
- [**Operator Guide**](docs/OPERATOR_GUIDE.md) — Monitoring and maintenance.
- [**Plugin Development**](docs/PLUGIN_DEVELOPMENT.md) — Building new Connectors.

## License
MIT. See [LICENSE](LICENSE) for details.
