#!/usr/bin/env python3
"""Add the job_log_completed_at_idx index for retention prune scans.

Idempotent. Safe to run on an active database; CREATE INDEX IF NOT EXISTS
acquires only a brief write lock and the index is small.

Wave 1 of the SQL tightening plan.
"""

import sqlite3
import sys
from pathlib import Path


INDEX_NAME = "job_log_completed_at_idx"
INDEX_DDL = (
    "CREATE INDEX IF NOT EXISTS job_log_completed_at_idx "
    "ON job_log(completed_at);"
)


def main() -> int:
    if len(sys.argv) != 2:
        print(
            "usage: scripts/migrate-job-log-completed-at-index.py <path-to-sqlite-db>",
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

        existed = conn.execute(
            "SELECT 1 FROM sqlite_master WHERE type='index' AND name=?;",
            (INDEX_NAME,),
        ).fetchone()

        conn.execute(INDEX_DDL)
        conn.commit()

        if existed:
            print(f"{INDEX_NAME}: already present (no-op)")
        else:
            print(f"{INDEX_NAME}: created")
        return 0
    finally:
        conn.close()


if __name__ == "__main__":
    sys.exit(main())
