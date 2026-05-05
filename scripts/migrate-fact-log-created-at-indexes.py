#!/usr/bin/env python3
"""Add created_at indexes on the three fact-log tables that grow without
prior retention: job_transitions, job_attempts, circuit_breaker_transitions.

Each table has an existing composite index leading with another column
(job_id or plugin/command), which doesn't support range scans on
created_at alone — required by the new PruneJobTransitions /
PruneJobAttempts / PruneBreakerTransitions functions.

Idempotent. Safe to run on an active database; CREATE INDEX IF NOT EXISTS
acquires only a brief write lock per table.

Wave 2 of the SQL tightening plan.
"""

import sqlite3
import sys
from pathlib import Path


INDEX_DDLS = [
    (
        "job_transitions_created_at_idx",
        "CREATE INDEX IF NOT EXISTS job_transitions_created_at_idx "
        "ON job_transitions(created_at);",
    ),
    (
        "job_attempts_created_at_idx",
        "CREATE INDEX IF NOT EXISTS job_attempts_created_at_idx "
        "ON job_attempts(created_at);",
    ),
    (
        "circuit_breaker_transitions_created_at_idx",
        "CREATE INDEX IF NOT EXISTS circuit_breaker_transitions_created_at_idx "
        "ON circuit_breaker_transitions(created_at);",
    ),
]


def main() -> int:
    if len(sys.argv) != 2:
        print(
            "usage: scripts/migrate-fact-log-created-at-indexes.py <path-to-sqlite-db>",
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

        for index_name, ddl in INDEX_DDLS:
            existed_before = conn.execute(
                "SELECT 1 FROM sqlite_master WHERE type='index' AND name=?;",
                (index_name,),
            ).fetchone()
            conn.execute(ddl)
            conn.commit()
            verb = "already present" if existed_before else "created"
            print(f"{index_name}: {verb}")
        return 0
    finally:
        conn.close()


if __name__ == "__main__":
    sys.exit(main())
