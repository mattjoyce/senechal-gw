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

## Payload

The JSON object attached to an event. Payload fields are passed to downstream plugins when the event is routed.

## Context (Event Context)

Accumulated payload fields stored across a pipeline chain. Used as a fallback when a downstream step needs information produced earlier.

## Result

A short human-readable summary returned by a plugin when `status=ok`. Included in the protocol response and propagated to downstream payloads when events are emitted.

## State Updates

The `state_updates` object in a plugin response. Applied as a shallow merge into the plugin's persistent state.

## Plugin State

A per-plugin JSON blob stored in SQLite that persists across runs.

## Job

The unit of execution. Every command invocation creates an immutable Job record capturing input, output, logs, and status.

## Queue

The persistent job queue stored in SQLite. Scheduler, router, API, and webhooks all enqueue jobs here.

## Schedule

A configuration entry that tells the scheduler when and how to run a plugin command.

## Schedule Entry

The scheduler's persisted state for a schedule: `last_fired_at`, `next_run_at`, `last_success_at`, and status (`active`, `paused`, `exhausted`).

## Scheduler

The heartbeat loop that evaluates schedules and enqueues due jobs.

## Cron

A five-field schedule expression (`min hour dom month dow`) used by cron schedules.

## Jitter

A random offset applied to interval schedules to avoid synchronized load.

## Catch-up

Startup policy that controls what happens when scheduled ticks were missed (`skip`, `run_once`, `run_all`).

## Overlap Policy

Per-schedule policy (`if_running`) that controls whether to skip, queue, or cancel when a previous job is still running.

## Dedupe Key

A key used to suppress duplicate enqueues within a dedupe window.

## Circuit Breaker

Scheduler guard that opens after repeated failures and temporarily blocks scheduled polls until a cooldown expires.

## Retry / Backoff

Automatic re-enqueue of failed jobs with exponential delay until max attempts are exhausted.

## Webhook

An HTTP endpoint that receives external events and enqueues plugin jobs.

## Route

A mapping from an event type to a downstream plugin or pipeline step.

## Token / Scope

Authentication credentials and permission sets used for API and webhook access.

## Baggage

Stateful metadata (JSON) that persists across the hops of a multi-step pipeline. Allows context — a user ID, a source URL, intermediate results — to travel with the execution rather than being re-fetched at each step.

## Execution Ledger

The persistent history of all jobs and pipeline transitions, stored in SQLite. Used for inspection, debugging, and lineage tracing.

## Workspace

An isolated directory created per job for file-based side effects — downloaded content, generated files, intermediate artifacts. Scoped to the job and retained for inspection.

## Skill

The machine-readable description of a plugin's capabilities, exported via `ductile system skills` or `GET /skills`. Useful for tooling and automated orchestration.
