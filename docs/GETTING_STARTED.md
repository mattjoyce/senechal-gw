# Getting Started with Ductile

Welcome to **Ductile**, a lightweight, reliable, and secure integration gateway designed for personal automation. This guide will help you get up and running in minutes.

---

## 1. Installation

Ductile is written in Go and requires version **1.25.4** or newer.

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/mattjoyce/ductile.git
    cd ductile
    ```

2.  **Build the gateway:**
    ```bash
    go build -o ductile ./cmd/ductile
    ```

    This creates a single executable named `ductile` in your project root.

---

## 2. Basic Usage (The Echo Showcase)

After building the binary, you can run the included `echo` plugin to verify the system.

### Step 1: Verify Plugin Discovery
Ductile expects a `plugins/` directory. Check that the echo manifest is present:
```bash
ls -F plugins/echo/manifest.yaml
```

### Step 2: Configure the Plugin
The `config.yaml` file in the root defines how plugins run. The `echo` plugin is enabled by default to run every 5 minutes.

```yaml
# config.yaml excerpt
plugins:
  echo:
    enabled: true
    schedule:
      every: 5m
      jitter: 30s
    config:
      message: "Hello from Ductile!"
```

### Step 3: Start the Gateway
Run the service in the foreground:
```bash
./ductile system start
```

You will see logs indicating the scheduler has started. After 5 minutes (or however you configured it), you'll see the echo job execute and complete.

### Step 4: Graceful Shutdown
Press `Ctrl+C` to stop the gateway. It will wait for any in-flight jobs to finish before releasing the process lock.

---

## 3. CLI Principles

Ductile is designed to be operated by both humans and LLMs. All commands follow a strict **NOUN ACTION** hierarchy:

-   **Hierarchy:** `ductile job inspect`, `ductile config lock`, `ductile system status`.
-   **Verbosity:** Use `-v` or `--verbose` for detailed logic traces.
-   **Safety:** Use `--dry-run` for any mutation to preview changes.
-   **Machine-Readability:** Use `--json` to get structured data for scripts or agents.

---

## Next Steps

-   **Operators:** Read the [Operator Guide](OPERATOR_GUIDE.md) to learn about monitoring and system maintenance.
-   **Developers:** Visit the [Plugin Development Guide](PLUGIN_DEVELOPMENT.md) to start building your own skills.
-   **Architects:** Deep dive into the [Architecture](ARCHITECTURE.md) and [Pipelines](PIPELINES.md) model.
