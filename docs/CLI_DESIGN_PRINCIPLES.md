# Senechal Gateway: CLI Design & Principles

**Version:** 1.0  
**Date:** 2026-02-12  
**Context:** RFC-004 (LLM as Operator/Admin)

This document defines the interface standards for the `senechal-gw` CLI. All commands must adhere to these principles to ensure safety, predictability, and machine-readability.

---

## 1. Core Philosophy: Intent over Implementation

Senechal is designed to be operated by both humans and LLMs. The CLI should speak the language of **Governance and Intent**, not mechanical implementation details.

### 1.1 NOUN VERB Hierarchy
All commands MUST follow a strict `NOUN VERB` pattern. 
*   **Good:** `senechal-gw job inspect`, `senechal-gw config seal`
*   **Bad:** `senechal-gw inspect-job`, `senechal-gw hash-update`

### 1.2 LLM-First Affordances
The CLI is the primary "API" for an LLM operator. It must provide:
*   **Predictable Output:** Consistent formatting across all commands.
*   **Structured Data:** Every "Inspect" or "List" command must support a `--json` flag.
*   **Exit Codes:** Strict adherence to standard exit codes (e.g., `EX_CONFIG` for validation failures).

---

## 2. Command Hierarchy

### 2.1 The Nouns (Resources)
*   `config`: The static definition of the system (YAML files, scopes, webhooks).
*   `pipeline`: The execution graph definitions.
*   `plugin`: The executable capabilities (discovery, status).
*   `job`: Instances of execution (lineage, logs, status).
*   `system`: Global gateway state (status, health, reload).

### 2.2 The Semantic Verbs (Intents)
*   **check**: Validate logic, syntax, and integrity (e.g., `config check`).
*   **lock**: Authorize current state by updating integrity manifests/hashes (e.g., `config lock`).
*   **get / set**: Retrieve or modify specific configuration nodes using a path syntax (e.g., `config set plugins.withings.enabled=true`).
*   **show / export**: Display the full, resolved, monolithic configuration or a specific entity node (e.g., `config show plugin:withings`).
*   **inspect**: Deep-dive into a specific runtime resource instance (e.g., `job inspect <id>`).
*   **list**: Show a summary of available resources.
*   **run / trigger**: Initiate a manual action or retry.

## 4. First-Class Entities

Senechal treats specific configuration blocks as "First-Class Entities." These are the primary objects an operator (Human or LLM) will interact with.

### 4.1 The Entities
*   **Plugin:** An executable capability (e.g., `plugin:withings`).
*   **Pipeline:** A defined workflow graph (e.g., `pipeline:video-wisdom`).
*   **Webhook:** An inbound HTTP endpoint (e.g., `webhook:github`).
*   **Token:** An API authorization key and its scopes (e.g., `token:admin-cli`).

### 4.2 CLI Intersection (Entity Addressing)
The CLI uses a standard `<entity_type>:<entity_name>` syntax to address these nodes. 

*   **Discovery:** `senechal-gw config show plugin:*` (Lists all plugins).
*   **Granularity:** `senechal-gw config show plugin:withings` (Displays only the Withings config block).
*   **Modification:** `senechal-gw config set plugin:withings.enabled=false`.

This pattern ensures that as the configuration grows, the operator can surgically target specific components without needing to understand the layout of the entire monolithic file.

---

## 5. Mandatory Flags

Every subcommand MUST implement these flags where relevant:

| Flag | Purpose | Requirement |
| :--- | :--- | :--- |
| `-v, --verbose` | Expose internal logic, path resolution, and baggage merges. | Mandatory for all. |
| `--dry-run` | Preview mutations without committing changes. | Mandatory for all "Write" verbs (`set`, `lock`, `run`). |
| `--json` | Return machine-readable structured data. | Mandatory for all "Read" verbs. |

---

## 4. Configuration Node Discovery

To support complex configurations, the `config` noun supports path-based matching and entity filtering.

### 4.1 Path Matching (`get` / `set`)
Use dot-notation to access specific values.
*   `senechal-gw config get plugins.withings.schedule.every`

### 4.2 Entity Filtering (`show`)
Use `<type>:<name>` syntax to isolate a first-class entity (Plugin, Pipeline, Webhook).
*   `senechal-gw config show plugin:withings`
*   `senechal-gw config show pipeline:video-wisdom`

This allows an LLM to "find any first class entity node that matches" without parsing a 2000-line monolithic YAML file.

---

## 5. Resource Mapping (Updated)

| Intent | Command | Status |
| :--- | :--- | :--- |
| **Verify Integrity** | `config check` | *Planned (Sprint 5)* |
| **Authorize Changes** | `config lock` | *Rename from hash-update* |
| **Get Value** | `config get <path>` | *Planned (Sprint 5)* |
| **Inspect Lineage** | `job inspect <id>` | *Implemented* |
| **Run Plugin** | `plugin run <name>` | *Refactor from run* |
| **System Status** | `system status` | *Refactor from status* |

---

## 5. Error Handling

Errors must be actionable. 
*   **Standard Errors:** Human-readable prose.
*   **JSON Errors:** If `--json` is set, errors must be returned as a JSON object: `{"error": "...", "code": 78, "context": {...}}`.
*   **Safety Envelope:** Commands that fail validation or dry-runs must return a non-zero exit code and prevent any physical side effects.
