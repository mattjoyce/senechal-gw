#!/usr/bin/env python3
"""Create Hickey Sprint 2 config snapshot schema on an existing Ductile DB.

This script mirrors the mono-schema additions for config snapshots. It is
intentionally additive and does not backfill speculative snapshot facts for
legacy jobs.
"""

import sqlite3
import sys
from pathlib import Path


SCHEMA_STATEMENTS = [
    """
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
    )
    """,
    """
    CREATE INDEX IF NOT EXISTS config_snapshots_loaded_at_idx
    ON config_snapshots(loaded_at)
    """,
    """
    CREATE INDEX IF NOT EXISTS job_queue_enqueued_config_snapshot_idx
    ON job_queue(enqueued_config_snapshot_id)
    """,
    """
    CREATE INDEX IF NOT EXISTS job_queue_started_config_snapshot_idx
    ON job_queue(started_config_snapshot_id)
    """,
    """
    CREATE INDEX IF NOT EXISTS job_log_enqueued_config_snapshot_idx
    ON job_log(enqueued_config_snapshot_id)
    """,
    """
    CREATE INDEX IF NOT EXISTS job_log_started_config_snapshot_idx
    ON job_log(started_config_snapshot_id)
    """,
]


TABLE_COLUMNS = [
    ("job_queue", "enqueued_config_snapshot_id", "TEXT"),
    ("job_queue", "started_config_snapshot_id", "TEXT"),
    ("job_log", "enqueued_config_snapshot_id", "TEXT"),
    ("job_log", "started_config_snapshot_id", "TEXT"),
]


def column_exists(conn: sqlite3.Connection, table: str, column: str) -> bool:
    rows = conn.execute(f"PRAGMA table_info({table})").fetchall()
    return any(row[1] == column for row in rows)


def ensure_column(conn: sqlite3.Connection, table: str, column: str, definition: str) -> None:
    if column_exists(conn, table, column):
        return
    conn.execute(f"ALTER TABLE {table} ADD COLUMN {column} {definition}")


def main() -> int:
    if len(sys.argv) != 2:
        print(
            "usage: scripts/migrate-hickey-sprint-2-config-snapshots.py <path-to-sqlite-db>",
            file=sys.stderr,
        )
        return 2

    db_path = Path(sys.argv[1]).expanduser().resolve()
    if not db_path.exists():
        print(f"db not found: {db_path}", file=sys.stderr)
        return 1

    conn = sqlite3.connect(str(db_path))
    try:
        conn.execute("PRAGMA foreign_keys=ON;")
        conn.execute("PRAGMA journal_mode=WAL;")
        conn.execute("PRAGMA busy_timeout=5000;")

        with conn:
            for stmt in SCHEMA_STATEMENTS[:2]:
                conn.execute(stmt)
            for table, column, definition in TABLE_COLUMNS:
                ensure_column(conn, table, column, definition)
            for stmt in SCHEMA_STATEMENTS[2:]:
                conn.execute(stmt)

        print(f"created Hickey Sprint 2 config snapshot schema in {db_path}")
        return 0
    finally:
        conn.close()


if __name__ == "__main__":
    raise SystemExit(main())
