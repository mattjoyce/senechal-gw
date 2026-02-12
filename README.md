# Senechal Gateway (senechal-gw)

[![Go Version](https://img.shields.io/badge/go-1.25.4-blue.svg)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**Senechal Gateway** is a lightweight, reliable, and secure integration gateway designed for personal automation. It acts as a digital steward, orchestrating tasks across various services using a simple, polyglot plugin architecture.

Built with resilience in mind, Senechal-GW ensures your automation workflows are executed reliably, even across system restarts or crashes.

## 🚀 Key Features

- **Polyglot Plugins:** Write logic in any language (Bash, Python, Go, Node.js). If it can read `stdin` and write `stdout`, it's a plugin.
- **Reliability First:** 
    - **Crash Recovery:** Automatically recovers and re-queues "orphaned" jobs after a crash or power failure.
    - **Circuit Breakers:** Protects your infrastructure and external APIs by failing fast when a plugin is consistently unhealthy.
- **Recursive Configuration:** Manage complex setups with nested YAML inclusions, relative path resolution, and environment variable interpolation.
- **Security by Design:**
    - **Integrity Verification:** BLAKE3-based hash checks for sensitive configuration files (like `tokens.yaml`).
    - **Scoped Auth:** Fine-grained Bearer token access (e.g., `plugin:ro` vs `plugin:rw`).
- **Management API:** A clean HTTP API and SSE (Server-Sent Events) stream for real-time monitoring and on-demand job triggering.

## 🧠 The Mental Model

Senechal-GW follows a **spawn-per-command** model. When a job is triggered (via the built-in Scheduler, an inbound Webhook, or the API), the gateway spawns a fresh process for the plugin.

1. **Scheduler/API/Webhook** enqueues a job.
2. **Dispatcher** spawns the plugin process.
3. **Communication** happens over a simple JSON-based protocol (v1).
4. **State** is persisted by the gateway, allowing plugins to stay stateless while maintaining operational continuity (e.g., OAuth tokens, last-run timestamps).

## 🛠 Quick Start

### 1. Installation
Requires Go 1.25.4+.

```bash
git clone https://github.com/mattjoyce/senechal-gw.git
cd senechal-gw
go build -o senechal-gw ./cmd/senechal-gw
```

### 2. Run the Echo Showcase
The included `echo` plugin demonstrates the protocol:

```bash
# Ensure plugins are discovered
./senechal-gw start
```

### 3. Trigger via API
```bash
# In another terminal, trigger the echo plugin manually
curl -X POST http://localhost:8080/trigger/echo/poll 
  -H "Authorization: Bearer ${YOUR_TOKEN}" 
  -H "Content-Type: application/json" 
  -d '{"message": "Hello Senechal!"}'
```

## 📝 Polyglot Showcase

### Bash Plugin (`run.sh`)
```bash
#!/bin/bash
REQUEST=$(cat)
MESSAGE=$(echo $REQUEST | jq -r '.config.message')
echo "{"status": "ok", "logs": [{"level": "info", "message": "$MESSAGE"}]}"
```

### Python Plugin (`run.py`)
```python
import sys, json
request = json.load(sys.stdin)
greeting = request['config'].get('greeting', 'Hello')
print(json.dumps({
    "status": "ok",
    "state_updates": {"last_seen": "now"},
    "logs": [{"level": "info", "message": f"{greeting} from Python"}]
}))
```

## 📚 Documentation

- [User Guide](docs/USER_GUIDE.md) - Installation, Configuration, and CLI usage.
- [Versioning](docs/VERSIONING.md) - SemVer policy and build metadata injection.
- [Plugin Development](docs/USER_GUIDE.md#10-plugin-development-guide) - Protocol details and examples.
- [Technical Specification](SPEC.md) - Deep dive into architecture and internals.
- [Architecture Decisions (RFCs)](RFC/) - Discussion on core design choices.

## ⚖️ License
MIT. See [LICENSE](LICENSE) for details. (Note: Project is in MVP stage).
