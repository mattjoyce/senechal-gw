-- Ductile SQLite Schema
-- This file is embedded into the binary and executed on startup.

-- Job Queue: Current active or pending jobs.
CREATE TABLE IF NOT EXISTS job_queue (
  id              TEXT PRIMARY KEY,
  plugin          TEXT NOT NULL,
  command         TEXT NOT NULL,
  payload         JSON,
  status          TEXT NOT NULL,
  attempt         INTEGER NOT NULL DEFAULT 1,
  max_attempts    INTEGER NOT NULL DEFAULT 4,
  submitted_by    TEXT NOT NULL,
  dedupe_key      TEXT,
  created_at      TEXT NOT NULL,
  started_at      TEXT,
  completed_at    TEXT,
  next_retry_at   TEXT,
  last_error      TEXT,
  parent_job_id   TEXT,
  source_event_id TEXT,
  event_context_id TEXT
);

CREATE INDEX IF NOT EXISTS job_queue_status_created_at_idx ON job_queue(status, created_at);
CREATE INDEX IF NOT EXISTS job_queue_plugin_command_status_idx ON job_queue(plugin, command, status);
CREATE UNIQUE INDEX IF NOT EXISTS job_queue_event_source_idx ON job_queue(parent_job_id, source_event_id) WHERE source_event_id IS NOT NULL;

-- Plugin State: Persistent key-value store for plugins.
CREATE TABLE IF NOT EXISTS plugin_state (
  plugin_name TEXT PRIMARY KEY,
  state       JSON NOT NULL DEFAULT '{}',
  updated_at  TEXT
);

-- Event Context: Pipeline execution history and data accumulation.
CREATE TABLE IF NOT EXISTS event_context (
  id               TEXT PRIMARY KEY,
  parent_id        TEXT,
  pipeline_name    TEXT,
  step_id          TEXT,
  accumulated_json JSON NOT NULL,
  created_at       TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS event_context_parent_id_idx ON event_context(parent_id);

-- Job Log: Historical record of completed jobs.
CREATE TABLE IF NOT EXISTS job_log (
  id              TEXT PRIMARY KEY,
  job_id          TEXT,
  plugin          TEXT NOT NULL,
  command         TEXT NOT NULL,
  status          TEXT NOT NULL,
  result          TEXT,
  attempt         INTEGER NOT NULL,
  submitted_by    TEXT NOT NULL,
  created_at      TEXT NOT NULL,
  completed_at    TEXT NOT NULL,
  last_error      TEXT,
  stderr          TEXT,
  parent_job_id   TEXT,
  source_event_id TEXT,
  event_context_id TEXT
);

CREATE INDEX IF NOT EXISTS job_log_job_id_attempt_idx ON job_log(job_id, attempt);

-- Circuit Breakers: Fault tolerance state.
CREATE TABLE IF NOT EXISTS circuit_breakers (
  plugin          TEXT NOT NULL,
  command         TEXT NOT NULL,
  state           TEXT NOT NULL DEFAULT 'closed',
  failure_count   INTEGER NOT NULL DEFAULT 0,
  opened_at       TEXT,
  last_failure_at TEXT,
  last_job_id     TEXT,
  updated_at      TEXT NOT NULL,
  PRIMARY KEY(plugin, command)
);

-- Schedule Entries: Last fire times and next scheduled runs.
CREATE TABLE IF NOT EXISTS schedule_entries (
  plugin          TEXT NOT NULL,
  schedule_id     TEXT NOT NULL,
  command         TEXT NOT NULL,
  status          TEXT NOT NULL DEFAULT 'active',
  reason          TEXT,
  last_fired_at   TEXT,
  last_success_job_id TEXT,
  last_success_at  TEXT,
  next_run_at      TEXT,
  updated_at      TEXT NOT NULL,
  PRIMARY KEY(plugin, schedule_id)
);

CREATE INDEX IF NOT EXISTS schedule_entries_status_idx ON schedule_entries(status);
