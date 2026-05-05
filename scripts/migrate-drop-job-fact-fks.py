#!/usr/bin/env python3
"""Drop FOREIGN KEY(job_id) REFERENCES job_queue(id) from job_transitions
and job_attempts.

The FK direction was structurally backwards: these tables are append-only
fact logs over job_queue's mutable status. Under foreign_keys=ON the FK
blocked PruneJobQueue (Wave 1) from deleting any terminal-state row that
had transitions or attempts — i.e., every row.

SQLite has no ALTER TABLE DROP CONSTRAINT, so this is the documented
table-rebuild ceremony. Idempotent: skips a table if its FK is already
gone.

The gateway must be quiesced before running this.

Usage:
    scripts/migrate-drop-job-fact-fks.py <db-path>
"""

import re
import sqlite3
import sys
from pathlib import Path


REBUILD_TABLES = {
    "job_transitions": """
CREATE TABLE job_transitions_new (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  job_id      TEXT NOT NULL,
  from_status TEXT,
  to_status   TEXT NOT NULL,
  reason      TEXT,
  created_at  TEXT NOT NULL
);
""",
    "job_attempts": """
CREATE TABLE job_attempts_new (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  job_id     TEXT NOT NULL,
  attempt    INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(job_id, attempt)
);
""",
}


def has_fk(conn: sqlite3.Connection, table: str) -> bool:
    """Check whether the table's CREATE statement still includes a FOREIGN KEY."""
    row = conn.execute(
        "SELECT sql FROM sqlite_master WHERE type='table' AND name=?;", (table,)
    ).fetchone()
    if row is None:
        raise RuntimeError(f"table {table!r} does not exist")
    return bool(re.search(r"\bFOREIGN\s+KEY\b", row[0], re.IGNORECASE))


def captured_indexes(conn: sqlite3.Connection, table: str) -> list[str]:
    """Return the original CREATE INDEX statements for the table."""
    rows = conn.execute(
        "SELECT sql FROM sqlite_master "
        "WHERE type='index' AND tbl_name=? AND sql IS NOT NULL;",
        (table,),
    ).fetchall()
    return [r[0] for r in rows]


def rebuild_one(conn: sqlite3.Connection, table: str, new_ddl: str) -> int:
    """Rebuild one table without its FK. Returns rowcount migrated."""
    indexes = captured_indexes(conn, table)
    conn.execute(new_ddl)
    cur = conn.execute(f"INSERT INTO {table}_new SELECT * FROM {table};")
    moved = cur.rowcount
    conn.execute(f"DROP TABLE {table};")
    conn.execute(f"ALTER TABLE {table}_new RENAME TO {table};")
    for ddl in indexes:
        conn.execute(ddl)
    return moved


def main() -> int:
    if len(sys.argv) != 2:
        print(
            "usage: scripts/migrate-drop-job-fact-fks.py <path-to-sqlite-db>",
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
        conn.execute("PRAGMA foreign_keys=OFF;")

        any_work = False
        conn.execute("BEGIN;")
        try:
            for table, ddl in REBUILD_TABLES.items():
                if not has_fk(conn, table):
                    print(f"{table}: FK already absent, skipping")
                    continue
                moved = rebuild_one(conn, table, ddl)
                print(f"{table}: rebuilt without FK, migrated {moved} rows")
                any_work = True

            if any_work:
                violations = conn.execute("PRAGMA foreign_key_check;").fetchall()
                if violations:
                    raise RuntimeError(
                        f"foreign_key_check found violations after rebuild: {violations}"
                    )
            conn.execute("COMMIT;")
        except Exception:
            conn.execute("ROLLBACK;")
            raise
        finally:
            conn.execute("PRAGMA foreign_keys=ON;")

        if not any_work:
            print("no rebuild needed; both tables already FK-free")
        return 0
    finally:
        conn.close()


if __name__ == "__main__":
    sys.exit(main())
