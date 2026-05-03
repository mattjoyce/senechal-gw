#!/usr/bin/env python3
"""One-time backfill: drain stale terminal-state rows from job_queue.

Pre-Wave-1 ductile builds had no DELETE on job_queue — terminal jobs
accumulated forever. This script deletes terminal-state rows older than
the chosen cutoff so the new per-tick PruneJobQueue starts from a clean
state on existing databases.

Default is dry-run: prints what would be deleted. Pass --apply to execute.
The gateway must be quiesced before --apply (PRAGMA integrity_check on a
live WAL is unsafe; the same caution applies to bulk deletes that need
fsync coherence with concurrent writers).

Wave 1 of the SQL tightening plan.

Usage:
    scripts/migrate-drain-stale-terminal-job-queue-rows.py <db-path>
    scripts/migrate-drain-stale-terminal-job-queue-rows.py <db-path> --apply
    scripts/migrate-drain-stale-terminal-job-queue-rows.py <db-path> --apply --older-than 24h
"""

import argparse
import re
import sqlite3
import sys
from pathlib import Path


TERMINAL_STATUSES = ("succeeded", "skipped", "failed", "timed_out", "dead")


def parse_duration(spec: str) -> str:
    """Convert "24h"/"7d"/"30m" into a SQLite datetime modifier."""
    match = re.fullmatch(r"(\d+)([smhd])", spec.strip())
    if not match:
        raise argparse.ArgumentTypeError(
            f"invalid duration {spec!r}; want N + unit s|m|h|d (e.g. 24h, 7d)"
        )
    n, unit = match.groups()
    unit_word = {"s": "seconds", "m": "minutes", "h": "hours", "d": "days"}[unit]
    return f"-{n} {unit_word}"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("db_path", help="path to ductile.db")
    parser.add_argument(
        "--older-than",
        type=parse_duration,
        default="24h",
        help="age cutoff (default 24h, matching service.job_queue_retention default)",
    )
    parser.add_argument(
        "--apply",
        action="store_true",
        help="execute the delete (default is dry-run that only counts)",
    )
    args = parser.parse_args()

    db_path = Path(args.db_path).expanduser().resolve()
    if not db_path.exists():
        print(f"db not found: {db_path}", file=sys.stderr)
        return 1

    placeholders = ",".join("?" * len(TERMINAL_STATUSES))
    select_q = f"""
SELECT status, COUNT(*) FROM job_queue
WHERE status IN ({placeholders})
  AND completed_at IS NOT NULL
  AND completed_at < datetime('now', ?)
GROUP BY status
ORDER BY status;
"""
    delete_q = f"""
DELETE FROM job_queue
WHERE status IN ({placeholders})
  AND completed_at IS NOT NULL
  AND completed_at < datetime('now', ?);
"""
    params = (*TERMINAL_STATUSES, args.older_than)

    conn = sqlite3.connect(str(db_path))
    try:
        conn.execute("PRAGMA journal_mode=WAL;")
        conn.execute("PRAGMA busy_timeout=5000;")

        rows = conn.execute(select_q, params).fetchall()
        total = sum(n for _, n in rows)
        print(f"db: {db_path}")
        print(f"cutoff: completed_at < now() {args.older_than}")
        print("matching rows by status:")
        if total == 0:
            print("  (none — nothing to drain)")
        else:
            for status, n in rows:
                print(f"  {status:>10}: {n}")
            print(f"  {'TOTAL':>10}: {total}")

        if not args.apply:
            print("dry-run — re-run with --apply to delete")
            return 0

        if total == 0:
            return 0

        cur = conn.execute(delete_q, params)
        conn.commit()
        print(f"deleted: {cur.rowcount}")
        return 0
    finally:
        conn.close()


if __name__ == "__main__":
    sys.exit(main())
