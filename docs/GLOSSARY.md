# Ductile Glossary

Key terms used throughout Ductile's documentation and configuration.

---

## Gateway

The `ductile` binary — the central runtime that manages plugins, schedules work, routes events, and maintains the execution ledger.

## Plugin

A polyglot adapter that connects Ductile to an external system (an API, a database, a shell command). Written in any language; communicates via JSON over stdin/stdout. Also called a **Connector**.

## Command

A discrete operation provided by a plugin. The four standard commands are:

- **`poll`** — proactive; Ductile calls the plugin on a schedule to pull data.
- **`handle`** — reactive; the plugin processes an incoming event.
- **`health`** — diagnostic; verifies the plugin's prerequisites are met.
- **`init`** — one-time setup; runs when a plugin is first registered.

## Pipeline

A configured sequence of steps triggered by an event. Pipelines define how data flows between multiple plugins to complete a workflow.

## Event

A typed packet of data (e.g., `file.changed`, `webhook.received`) that signals something happened and triggers routing within the gateway.

## Job

The unit of execution. Every command invocation creates an immutable Job record capturing input, output, logs, and status.

## Baggage

Stateful metadata (JSON) that persists across the hops of a multi-step pipeline. Allows context — a user ID, a source URL, intermediate results — to travel with the execution rather than being re-fetched at each step.

## Execution Ledger

The persistent history of all jobs and pipeline transitions, stored in SQLite. Used for inspection, debugging, and lineage tracing.

## Workspace

An isolated directory created per job for file-based side effects — downloaded content, generated files, intermediate artifacts. Scoped to the job and retained for inspection.

## Skill

The machine-readable description of a plugin's capabilities, exported via `ductile system skills` or `GET /skills`. Useful for tooling and automated orchestration.
