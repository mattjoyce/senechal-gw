#!/usr/bin/env python3
import sqlite3
import sys
from pathlib import Path


def parse_job_id(log_id: str) -> str | None:
    idx = log_id.rfind("-")
    if idx <= 0 or idx == len(log_id) - 1:
        return None
    return log_id[:idx]


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: scripts/migrate-job-log-job-id.py <path-to-sqlite-db>", file=sys.stderr)
        return 2

    db_path = Path(sys.argv[1]).expanduser().resolve()
    if not db_path.exists():
        print(f"db not found: {db_path}", file=sys.stderr)
        return 1

    conn = sqlite3.connect(str(db_path))
    try:
        conn.execute("PRAGMA journal_mode=WAL;")
        conn.execute("PRAGMA busy_timeout=5000;")

        cols = {row[1] for row in conn.execute("PRAGMA table_info(job_log)")}
        if "job_id" not in cols:
            conn.execute("ALTER TABLE job_log ADD COLUMN job_id TEXT")

        rows = list(conn.execute("SELECT id FROM job_log WHERE job_id IS NULL OR job_id = ''"))
        updates: list[tuple[str, str]] = []
        for (log_id,) in rows:
            job_id = parse_job_id(log_id)
            if job_id is None:
                continue
            updates.append((job_id, log_id))

        with conn:
            if updates:
                conn.executemany("UPDATE job_log SET job_id = ? WHERE id = ?", updates)
            conn.execute("CREATE INDEX IF NOT EXISTS job_log_job_id_attempt_idx ON job_log(job_id, attempt)")

        print(f"migrated {len(updates)} job_log rows in {db_path}")
        return 0
    finally:
        conn.close()


if __name__ == "__main__":
    raise SystemExit(main())
