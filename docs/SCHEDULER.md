# Scheduler

Detailed reference for Ductile's scheduler behavior and schedule configuration.

## Overview

The scheduler runs a single heartbeat loop. On each tick it evaluates all enabled plugin schedules and enqueues due jobs. Each schedule entry is tracked independently in the `schedule_entries` table so catch-up and next-run behavior are stable across restarts.

Jobs are always enqueued as the schedule's `command` (default: `poll`) and include the schedule's `payload`.

## Schedule Entry Fields

```yaml
plugins:
  example:
    schedules:
      - id: hourly
        command: poll
        every: 1h
        jitter: 30s
        catch_up: run_once
        if_running: skip
        only_between: "08:00-18:00"
        timezone: "Australia/Sydney"
        not_on: [saturday, sunday]
        payload:
          source: scheduler
```

### Common Fields
- `id`: Unique schedule ID within the plugin (default: `default`).
- `command`: Command to run (default: `poll`).
- `payload`: JSON object merged into the command payload.

### Schedule Types
Exactly one of the following should be set:
- `every`: Interval schedule (supports `5m`, `15m`, `30m`, `hourly`, `2h`, `daily`, `weekly`, `monthly`).
- `cron`: Standard 5-field cron (`min hour dom month dow`).
- `at`: One-shot RFC3339 timestamp (UTC or offset).
- `after`: One-shot delay from service start (duration).

## Time Constraints

These constraints are applied before enqueueing a due job.

- `jitter`: Random offset applied to interval schedules per run.
- `only_between`: Time window in local schedule time (e.g. `"08:00-22:00"`).
  - Supports overnight windows such as `"22:00-06:00"`.
- `timezone`: IANA timezone used for cron and time window evaluation.
- `not_on`: Weekdays to skip (string names like `saturday` or integers `0-6`, `7` for Sunday).

## Catch-up Policy

On startup, the scheduler can run missed ticks based on `catch_up`:
- `skip` (default): Ignore missed intervals.
- `run_once`: Enqueue a single catch-up job if any ticks were missed.
- `run_all`: Enqueue one job per missed interval (bounded to 100 runs).

Catch-up applies only to `every` schedules. Catch-up jobs use a `catchup`-scoped dedupe key to avoid duplication.

## Overlap Policy

`if_running` controls what happens when a prior job is still in-flight:
- `skip` (default): Do not enqueue a new job.
- `queue`: Enqueue regardless of in-flight jobs.
- `cancel`: Cancel outstanding jobs for the same plugin/command, then enqueue.

## Poll Guard

A global per-plugin guard prevents multiple concurrent scheduled polls:

```yaml
plugins:
  example:
    max_outstanding_polls: 1
```

If a matching `queued` or `running` job exists, the scheduler skips enqueueing.

## Circuit Breaker

Scheduler-originated polls respect the circuit breaker:
- Opens after `threshold` consecutive failures.
- Remains open for `reset_after`.
- Half-open probe allows one poll; success closes the circuit.
- Current state is stored in `circuit_breakers`; append-only history is stored in `circuit_breaker_transitions`.
- Operators can inspect history with `ductile system breaker <plugin> [--json]`.

```yaml
plugins:
  example:
    circuit_breaker:
      threshold: 3
      reset_after: 30m
```

## State Tracking

Schedule state is stored in `schedule_entries`:
- `last_fired_at`: Last time the scheduler attempted to enqueue.
- `last_success_at` / `last_success_job_id`: Latest successful run.
- `next_run_at`: Next due timestamp.
- `status`: `active`, `paused_invalid`, `paused_manual`, `exhausted`.

One-shot schedules (`at`, `after`) transition to `exhausted` after firing.

## Examples

### Cron with timezone
```yaml
plugins:
  reports:
    schedules:
      - id: weekdays-9am
        cron: "0 9 * * 1-5"
        timezone: "Australia/Sydney"
```

### One-shot at
```yaml
plugins:
  reminder:
    schedules:
      - id: send-once
        at: "2026-03-15T14:00:00Z"
```

### Only between + not_on
```yaml
plugins:
  poller:
    schedules:
      - every: 5m
        only_between: "08:00-18:00"
        not_on: [saturday, sunday]
```
