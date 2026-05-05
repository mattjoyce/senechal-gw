#!/usr/bin/env python3
"""Drop nine indexes with no production backing query (Wave 2.E).

Each index was verified by grep across internal/ and cmd/ductile/ — no
WHERE/ORDER BY/JOIN clause uses the column or composite the index covers.
Detailed audit lives in MEMORY/WORK/...sql-tightening-plan/PRD.md iteration 8.

Idempotent. DROP INDEX IF EXISTS is a SQLite metadata operation; no table
rebuild, hot-safe under an active gateway. Reversible — re-add any single
index with a one-line CREATE INDEX migration if a future query needs it.

Wave 2.E of the SQL tightening plan.
"""

import sqlite3
import sys
from pathlib import Path


# Each entry: (index_name, table_name) — table is informational only.
DROPS = [
    ("job_queue_enqueued_config_snapshot_idx", "job_queue"),
    ("job_queue_started_config_snapshot_idx", "job_queue"),
    ("config_snapshots_loaded_at_idx", "config_snapshots"),
    ("plugin_facts_job_id_idx", "plugin_facts"),
    ("event_context_parent_id_idx", "event_context"),
    ("job_log_enqueued_config_snapshot_idx", "job_log"),
    ("job_log_started_config_snapshot_idx", "job_log"),
    ("circuit_breaker_transitions_job_idx", "circuit_breaker_transitions"),
    ("schedule_entries_status_idx", "schedule_entries"),
]


def main() -> int:
    if len(sys.argv) != 2:
        print(
            "usage: scripts/migrate-drop-unused-indexes.py <path-to-sqlite-db>",
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

        dropped = 0
        absent = 0
        for index_name, table_name in DROPS:
            existed = conn.execute(
                "SELECT 1 FROM sqlite_master WHERE type='index' AND name=?;",
                (index_name,),
            ).fetchone()
            conn.execute(f'DROP INDEX IF EXISTS "{index_name}";')
            if existed:
                print(f"{index_name} on {table_name}: dropped")
                dropped += 1
            else:
                print(f"{index_name} on {table_name}: already absent (no-op)")
                absent += 1
        conn.commit()

        print(f"\nsummary: dropped={dropped} absent={absent} total={len(DROPS)}")
        return 0
    finally:
        conn.close()


if __name__ == "__main__":
    sys.exit(main())
