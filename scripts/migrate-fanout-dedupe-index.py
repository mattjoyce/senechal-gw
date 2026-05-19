#!/usr/bin/env python3
"""Widen job_queue_event_source_idx to include the fan-out target (C-FRO-16).

The old unique index was:
  job_queue_event_source_idx
    ON job_queue(parent_job_id, source_event_id) WHERE source_event_id IS NOT NULL

All fan-out siblings of one source event share parent_job_id + source_event_id,
so INSERT OR IGNORE collapsed every sibling after the first into a silent
no-op (C-FRO-16: silent work loss). The new index adds the target dimension:
  job_queue_event_source_idx
    ON job_queue(parent_job_id, source_event_id, plugin, command)
       WHERE source_event_id IS NOT NULL

Distinct fan-out targets now coexist; genuine same-target redelivery still
dedupes (at-least-once idempotency preserved). schema.sql is CREATE ... IF NOT
EXISTS on startup, so existing databases keep the OLD index until this script
runs. There is no migration framework — this follows the scripts/migrate-*.py
pattern.

Idempotent. Drops then recreates by exact definition; re-running is a no-op
once the new shape is in place. Index DDL is a SQLite metadata operation (no
table rebuild), hot-safe under an active gateway.
"""

import sqlite3
import sys
from pathlib import Path

INDEX_NAME = "job_queue_event_source_idx"
NEW_DEF = (
    f'CREATE UNIQUE INDEX "{INDEX_NAME}" '
    "ON job_queue(parent_job_id, source_event_id, plugin, command) "
    "WHERE source_event_id IS NOT NULL"
)


def main() -> int:
    if len(sys.argv) != 2:
        print(
            "usage: scripts/migrate-fanout-dedupe-index.py <path-to-sqlite-db>",
            file=sys.stderr,
        )
        return 2

    db_path = Path(sys.argv[1]).expanduser().resolve()
    if not db_path.exists():
        print(f"db not found: {db_path}", file=sys.stderr)
        return 1

    conn = sqlite3.connect(str(db_path))
    try:
        conn.execute("PRAGMA journal_mode=WAL;")
        conn.execute("PRAGMA busy_timeout=5000;")

        current = conn.execute(
            "SELECT sql FROM sqlite_master WHERE type='index' AND name=?;",
            (INDEX_NAME,),
        ).fetchone()

        if current and current[0] and "command" in current[0].lower():
            print(f"{INDEX_NAME}: already widened (no-op)")
            return 0

        if current:
            conn.execute(f'DROP INDEX "{INDEX_NAME}";')
            print(f"{INDEX_NAME}: dropped old (parent, source_event) index")
        else:
            print(f"{INDEX_NAME}: not present, creating fresh")

        conn.execute(NEW_DEF + ";")
        conn.commit()
        print(
            f"{INDEX_NAME}: created (parent_job_id, source_event_id, plugin, "
            "command) WHERE source_event_id IS NOT NULL"
        )
        return 0
    finally:
        conn.close()


if __name__ == "__main__":
    sys.exit(main())
