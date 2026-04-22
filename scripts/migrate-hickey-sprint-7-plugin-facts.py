#!/usr/bin/env python3
"""Create Hickey Sprint 7 plugin facts schema on an existing Ductile DB.

This script mirrors the mono-schema additions for append-only plugin facts.
It is intentionally additive and does not backfill speculative facts for
legacy plugin runs.
"""

import sqlite3
import sys
from pathlib import Path


REQUIRED_BASE_COLUMNS = {
    "plugin_state": {"plugin_name", "state"},
    "job_queue": {"id", "plugin", "command", "status"},
}

PLUGIN_FACTS_COLUMNS = {
    "id": {"type": "TEXT", "pk": True},
    "plugin_name": {"type": "TEXT", "notnull": True},
    "fact_type": {"type": "TEXT", "notnull": True},
    "job_id": {"type": "TEXT", "notnull": True},
    "command": {"type": "TEXT", "notnull": True},
    "fact_json": {"type": "JSON", "notnull": True},
    "created_at": {"type": "TEXT", "notnull": True},
}

REQUIRED_INDEXES = {
    "plugin_facts_plugin_created_at_idx": ("plugin_facts", ["plugin_name", "created_at"]),
    "plugin_facts_plugin_type_created_at_idx": (
        "plugin_facts",
        ["plugin_name", "fact_type", "created_at"],
    ),
    "plugin_facts_job_id_idx": ("plugin_facts", ["job_id"]),
}

SCHEMA_STATEMENTS = [
    """
    CREATE TABLE IF NOT EXISTS plugin_facts (
      id          TEXT PRIMARY KEY,
      plugin_name TEXT NOT NULL,
      fact_type   TEXT NOT NULL,
      job_id      TEXT NOT NULL,
      command     TEXT NOT NULL,
      fact_json   JSON NOT NULL,
      created_at  TEXT NOT NULL
    )
    """,
    """
    CREATE INDEX IF NOT EXISTS plugin_facts_plugin_created_at_idx
    ON plugin_facts(plugin_name, created_at)
    """,
    """
    CREATE INDEX IF NOT EXISTS plugin_facts_plugin_type_created_at_idx
    ON plugin_facts(plugin_name, fact_type, created_at)
    """,
    """
    CREATE INDEX IF NOT EXISTS plugin_facts_job_id_idx
    ON plugin_facts(job_id)
    """,
]


def fail(message: str) -> None:
    raise RuntimeError(message)


def normalize_type(value: object) -> str:
    return str(value or "").strip().upper()


def table_exists(conn: sqlite3.Connection, table: str) -> bool:
    row = conn.execute(
        "SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", (table,)
    ).fetchone()
    return row is not None


def table_columns(conn: sqlite3.Connection, table: str) -> dict[str, dict[str, object]]:
    rows = conn.execute(f"PRAGMA table_info({table})").fetchall()
    return {
        row[1]: {
            "cid": row[0],
            "name": row[1],
            "type": normalize_type(row[2]),
            "notnull": bool(row[3]),
            "default": row[4],
            "pk": bool(row[5]),
        }
        for row in rows
    }


def validate_preflight(conn: sqlite3.Connection) -> None:
    for table, required_columns in REQUIRED_BASE_COLUMNS.items():
        if not table_exists(conn, table):
            fail(f"required table {table!r} is missing; is this a Ductile state DB?")
        columns = set(table_columns(conn, table))
        missing = sorted(required_columns - columns)
        if missing:
            fail(f"required table {table!r} is missing columns: {', '.join(missing)}")

    if table_exists(conn, "plugin_facts"):
        validate_plugin_facts_table(conn)


def validate_plugin_facts_table(conn: sqlite3.Connection) -> None:
    if not table_exists(conn, "plugin_facts"):
        fail("plugin_facts table is missing")

    columns = table_columns(conn, "plugin_facts")
    for column, expected in PLUGIN_FACTS_COLUMNS.items():
        if column not in columns:
            fail(f"plugin_facts.{column} is missing")
        actual = columns[column]
        expected_type = normalize_type(expected["type"])
        if actual["type"] != expected_type:
            fail(
                f"plugin_facts.{column} has type {actual['type']!r}, "
                f"expected {expected_type!r}"
            )
        if expected.get("pk") and not actual["pk"]:
            fail(f"plugin_facts.{column} is not a primary key column")
        if expected.get("notnull") and not actual["notnull"]:
            fail(f"plugin_facts.{column} is nullable but must be NOT NULL")


def validate_index(conn: sqlite3.Connection, index_name: str, table: str, columns: list[str]) -> None:
    row = conn.execute(
        "SELECT tbl_name FROM sqlite_master WHERE type = 'index' AND name = ?",
        (index_name,),
    ).fetchone()
    if row is None:
        fail(f"required index {index_name!r} is missing")
    if row[0] != table:
        fail(f"index {index_name!r} is on table {row[0]!r}, expected {table!r}")

    index_columns = [
        row[2] for row in conn.execute(f"PRAGMA index_info({index_name})").fetchall()
    ]
    if index_columns != columns:
        fail(f"index {index_name!r} covers {index_columns!r}, expected {columns!r}")


def validate_final_schema(conn: sqlite3.Connection) -> None:
    validate_plugin_facts_table(conn)
    for index_name, (table, columns) in REQUIRED_INDEXES.items():
        validate_index(conn, index_name, table, columns)


def main() -> int:
    if len(sys.argv) != 2:
        print(
            "usage: scripts/migrate-hickey-sprint-7-plugin-facts.py <path-to-sqlite-db>",
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

        validate_preflight(conn)

        with conn:
            for stmt in SCHEMA_STATEMENTS:
                conn.execute(stmt)

        validate_final_schema(conn)

        print(f"verified Hickey Sprint 7 plugin facts schema in {db_path}")
        return 0
    except RuntimeError as err:
        print(f"migration failed: {err}", file=sys.stderr)
        return 1
    finally:
        conn.close()


if __name__ == "__main__":
    raise SystemExit(main())
