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
  event_context_id TEXT,
  enqueued_config_snapshot_id TEXT,
  started_config_snapshot_id TEXT
);

CREATE INDEX IF NOT EXISTS job_queue_status_created_at_idx ON job_queue(status, created_at);
CREATE INDEX IF NOT EXISTS job_queue_plugin_command_status_idx ON job_queue(plugin, command, status);
CREATE INDEX IF NOT EXISTS job_queue_dedupe_status_completed_idx ON job_queue(dedupe_key, status, completed_at);
CREATE UNIQUE INDEX IF NOT EXISTS job_queue_event_source_idx ON job_queue(parent_job_id, source_event_id) WHERE source_event_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS job_queue_enqueued_config_snapshot_idx ON job_queue(enqueued_config_snapshot_id);
CREATE INDEX IF NOT EXISTS job_queue_started_config_snapshot_idx ON job_queue(started_config_snapshot_id);

-- Hickey Sprint 1 branch hickey-sprint-1-job-lineage:
-- append-only job lineage facts. job_queue.status and job_queue.attempt
-- remain the compatibility/cache fields.
CREATE TABLE IF NOT EXISTS job_transitions (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  job_id      TEXT NOT NULL,
  from_status TEXT,
  to_status   TEXT NOT NULL,
  reason      TEXT,
  created_at  TEXT NOT NULL,
  FOREIGN KEY(job_id) REFERENCES job_queue(id)
);

CREATE INDEX IF NOT EXISTS job_transitions_job_created_at_idx ON job_transitions(job_id, created_at);

CREATE TABLE IF NOT EXISTS job_attempts (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  job_id     TEXT NOT NULL,
  attempt    INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  FOREIGN KEY(job_id) REFERENCES job_queue(id),
  UNIQUE(job_id, attempt)
);

CREATE INDEX IF NOT EXISTS job_attempts_job_created_at_idx ON job_attempts(job_id, created_at);

-- Hickey Sprint 2 config snapshots:
-- append-only records of successful active runtime config values. Job rows
-- reference the config value that admitted them and the config value that
-- actually started execution. Existing rows may have NULL snapshot IDs.
CREATE TABLE IF NOT EXISTS config_snapshots (
  id                  TEXT PRIMARY KEY,
  config_hash         TEXT NOT NULL,
  source_hash         TEXT,
  source_path         TEXT,
  source              TEXT,
  reason              TEXT NOT NULL,
  loaded_at           TEXT NOT NULL,
  ductile_version     TEXT,
  binary_path         TEXT,
  snapshot_format     INTEGER NOT NULL DEFAULT 1,
  semantics           JSON,
  plugin_fingerprints JSON,
  sanitized_config    JSON,
  secret_fingerprints JSON
);

CREATE INDEX IF NOT EXISTS config_snapshots_loaded_at_idx ON config_snapshots(loaded_at);

-- Plugin State: Persistent key-value store for plugins.
CREATE TABLE IF NOT EXISTS plugin_state (
  plugin_name TEXT PRIMARY KEY,
  state       JSON NOT NULL DEFAULT '{}',
  updated_at  TEXT
);

-- Hickey Sprint 7 plugin facts:
-- append-only plugin observations. plugin_state remains the
-- compatibility/current-state row for legacy plugin state reads.
CREATE TABLE IF NOT EXISTS plugin_facts (
  id          TEXT PRIMARY KEY,
  plugin_name TEXT NOT NULL,
  fact_type   TEXT NOT NULL,
  job_id      TEXT NOT NULL,
  command     TEXT NOT NULL,
  fact_json   JSON NOT NULL,
  created_at  TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS plugin_facts_plugin_created_at_idx
ON plugin_facts(plugin_name, created_at);

CREATE INDEX IF NOT EXISTS plugin_facts_plugin_type_created_at_idx
ON plugin_facts(plugin_name, fact_type, created_at);

CREATE INDEX IF NOT EXISTS plugin_facts_job_id_idx
ON plugin_facts(job_id);

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
  event_context_id TEXT,
  enqueued_config_snapshot_id TEXT,
  started_config_snapshot_id TEXT
);

CREATE INDEX IF NOT EXISTS job_log_job_id_attempt_idx ON job_log(job_id, attempt);
CREATE INDEX IF NOT EXISTS job_log_enqueued_config_snapshot_idx ON job_log(enqueued_config_snapshot_id);
CREATE INDEX IF NOT EXISTS job_log_started_config_snapshot_idx ON job_log(started_config_snapshot_id);

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

-- Hickey Sprint 4 runtime truth cleanup:
-- append-only circuit breaker history. circuit_breakers remains the
-- compatibility/current-state row used by scheduler decisions.
CREATE TABLE IF NOT EXISTS circuit_breaker_transitions (
  id            TEXT PRIMARY KEY,
  plugin        TEXT NOT NULL,
  command       TEXT NOT NULL,
  from_state    TEXT,
  to_state      TEXT NOT NULL,
  failure_count INTEGER NOT NULL DEFAULT 0,
  reason        TEXT NOT NULL,
  job_id        TEXT,
  created_at    TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS circuit_breaker_transitions_plugin_command_created_idx
ON circuit_breaker_transitions(plugin, command, created_at);

CREATE INDEX IF NOT EXISTS circuit_breaker_transitions_job_idx
ON circuit_breaker_transitions(job_id)
WHERE job_id IS NOT NULL;

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
