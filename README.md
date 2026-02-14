# Ductile (ductile)

[![Go Version](https://img.shields.io/badge/go-1.25.4-blue.svg)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**Ductile** is a lightweight, reliable, and secure integration gateway designed for personal automation. It acts as a digital steward, orchestrating tasks across various services using a simple, polyglot plugin architecture.

Built with resilience in mind, Ductile ensures your automation workflows are executed reliably, even across system restarts or crashes.

---

## 🚀 Documentation Index

We have reorganized our documentation into role-based manuals to improve clarity for both humans and LLM operators.

### 🏁 [Getting Started](docs/GETTING_STARTED.md)
The absolute first stop. Installation, basic usage, and core CLI principles.

### ⚙️ [Operator Guide](docs/OPERATOR_GUIDE.md)
The manual for running the system. Monitoring (TUI), system maintenance, and API control.

### 🛠️ [Plugin Development](docs/PLUGIN_DEVELOPMENT.md)
A guide for developers building new capabilities (Skills) using Protocol v2.

### 📐 [Architecture & Pipelines](docs/ARCHITECTURE.md)
Deep dive into the engine, the Governance Hybrid model, and the Pipeline DSL.

### 📜 [Configuration Reference](docs/CONFIG_REFERENCE.md)
Strict technical specification for YAML structure and integrity verification.

---

## 🧠 Key Features

- **Polyglot Plugins:** Write logic in any language. If it can read `stdin` and write `stdout`, it's a plugin.
- **Reliability First:** Crash recovery, exponential backoff, and circuit breakers (Sprint 4).
- **Governance Hybrid:** Automatic metadata ("Baggage") accumulation and zero-copy workspace cloning.
- **Security by Design:** BLAKE3-based integrity verification and scoped Bearer tokens.
- **Real-Time Monitoring:** SSE event stream and a built-in "btop-style" TUI dashboard.

## 🛠 Quick Start

```bash
# Build
go build -o ductile ./cmd/ductile

# Start the gateway
./ductile system start

# Launch the monitor (in another terminal)
./ductile system monitor --api-key "your-token"
```

## ⚖️ License
MIT. See [LICENSE](LICENSE) for details. (Note: Project is in MVP stage).
