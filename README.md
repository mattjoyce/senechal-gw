# Ductile (ductile)

[![Go Version](https://img.shields.io/badge/go-1.25.4-blue.svg)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**Ductile is a lightweight, open-source integration engine for the agentic era.**

It sits in the **"Integration Sphere,"** grounding high-level model intent into safe, functional, and observable system side effects.

---

## What is Ductile?

Ductile is a governed execution gateway designed to be operated by both humans and LLM Operators. It provides the functional **affordances** (Skills) that an agent needs to be useful, while maintaining **robust compound semantic grounding** that a human systems engineer can trust.

### Core Philosophy: The Managed Integration Gateway

- **Lightweight & Quick to Deploy:** A single-binary Go runtime with SQLite persistence. No complex infrastructure required.
- **Polyglot Extensibility:** Build connectors in any language. If it can read `stdin` and write `stdout`, it's a Ductile plugin.
- **Agentic Readiness (LX):** Self-describing via the `--skills` protocol, governable via RFC-004 (Config Lock), and built for "Inference Frugality."
- **Reliability First:** Built-in retries, circuit breakers, and stateful **Baggage** for tracing complex multi-hop orchestrations.
- **Audit-First:** Every action produces an immutable record in the **Execution Ledger**, complete with workspace artifacts and lineage.

---

## Key Concepts (The Integration Codex)

| Term | Integration Role (Core) | Agentic Context (LX) |
| :--- | :--- | :--- |
| **Gateway** | The runtime engine (Ductile). | The "Nervous System" of the host. |
| **Plugin** | A **Connector** to an external system. | The source of new capabilities. |
| **Command** | A discrete **Operation** (poll, handle). | The specific **Skill** exposed to the LLM. |
| **Pipeline** | A multi-hop **Orchestration**. | A governed, complex reasoning chain. |
| **Baggage** | **Stateful Metadata** (Tracing). | The LLM's "Working Memory". |
| **Workspace** | **Isolated Execution** environment. | The "Paper Trail" for auditing. |

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

## Documentation Index

- [**Getting Started**](docs/GETTING_STARTED.md) — Installation and basic usage.
- [**The Glossary**](docs/GLOSSARY.md) — Nomenclature of the Integration Codex.
- [**Cookbook**](docs/COOKBOOK.md) — Common patterns (Discord, YouTube, Astro).
- [**Core Mechanics**](docs/ARCHITECTURE.md) — Deep dive into the Integration Sphere.
- [**Operator Guide**](docs/OPERATOR_GUIDE.md) — Monitoring and maintenance.
- [**Plugin Development**](docs/PLUGIN_DEVELOPMENT.md) — Building new Connectors.

## License
MIT. See [LICENSE](LICENSE) for details.
