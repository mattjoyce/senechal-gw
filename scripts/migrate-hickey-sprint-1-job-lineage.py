#!/usr/bin/env python3
"""Create Hickey Sprint 1 job lineage tables on an existing Ductile DB.

Branch link: hickey-sprint-1-job-lineage.

This script mirrors the mono-schema additions for the Hickey Sprint 1
execution-lineage work. It is intentionally additive and does not backfill
speculative facts for legacy jobs.
"""

import sqlite3
import sys
from pathlib import Path


SCHEMA_STATEMENTS = [
    """
    CREATE TABLE IF NOT EXISTS job_transitions (
      id          INTEGER PRIMARY KEY AUTOINCREMENT,
      job_id      TEXT NOT NULL,
      from_status TEXT,
      to_status   TEXT NOT NULL,
      reason      TEXT,
      created_at  TEXT NOT NULL,
      FOREIGN KEY(job_id) REFERENCES job_queue(id)
    )
    """,
    """
    CREATE INDEX IF NOT EXISTS job_transitions_job_created_at_idx
    ON job_transitions(job_id, created_at)
    """,
    """
    CREATE TABLE IF NOT EXISTS job_attempts (
      id         INTEGER PRIMARY KEY AUTOINCREMENT,
      job_id     TEXT NOT NULL,
      attempt    INTEGER NOT NULL,
      created_at TEXT NOT NULL,
      FOREIGN KEY(job_id) REFERENCES job_queue(id),
      UNIQUE(job_id, attempt)
    )
    """,
    """
    CREATE INDEX IF NOT EXISTS job_attempts_job_created_at_idx
    ON job_attempts(job_id, created_at)
    """,
]


def main() -> int:
    if len(sys.argv) != 2:
        print(
            "usage: scripts/migrate-hickey-sprint-1-job-lineage.py <path-to-sqlite-db>",
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
            for stmt in SCHEMA_STATEMENTS:
                conn.execute(stmt)

        print(f"created Hickey Sprint 1 job lineage tables in {db_path}")
        return 0
    finally:
        conn.close()


if __name__ == "__main__":
    raise SystemExit(main())
