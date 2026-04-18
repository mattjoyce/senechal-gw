#!/usr/bin/env python3
"""Create Hickey Sprint 2 config snapshot schema on an existing Ductile DB.

This script mirrors the mono-schema additions for config snapshots. It is
intentionally additive and does not backfill speculative snapshot facts for
legacy jobs.
"""

import sqlite3
import sys
from pathlib import Path


REQUIRED_BASE_COLUMNS = {
    "job_queue": {"id", "plugin", "command", "status"},
    "job_log": {"id", "plugin", "command", "status"},
}

CONFIG_SNAPSHOT_COLUMNS = {
    "id": {"type": "TEXT", "pk": True},
    "config_hash": {"type": "TEXT", "notnull": True},
    "source_hash": {"type": "TEXT"},
    "source_path": {"type": "TEXT"},
    "source": {"type": "TEXT"},
    "reason": {"type": "TEXT", "notnull": True},
    "loaded_at": {"type": "TEXT", "notnull": True},
    "ductile_version": {"type": "TEXT"},
    "binary_path": {"type": "TEXT"},
    "snapshot_format": {"type": "INTEGER", "notnull": True, "default": "1"},
    "semantics": {"type": "JSON"},
    "plugin_fingerprints": {"type": "JSON"},
    "sanitized_config": {"type": "JSON"},
    "secret_fingerprints": {"type": "JSON"},
}

JOB_SNAPSHOT_COLUMNS = {
    "enqueued_config_snapshot_id": "TEXT",
    "started_config_snapshot_id": "TEXT",
}

REQUIRED_INDEXES = {
    "config_snapshots_loaded_at_idx": ("config_snapshots", ["loaded_at"]),
    "job_queue_enqueued_config_snapshot_idx": (
        "job_queue",
        ["enqueued_config_snapshot_id"],
    ),
    "job_queue_started_config_snapshot_idx": (
        "job_queue",
        ["started_config_snapshot_id"],
    ),
    "job_log_enqueued_config_snapshot_idx": (
        "job_log",
        ["enqueued_config_snapshot_id"],
    ),
    "job_log_started_config_snapshot_idx": (
        "job_log",
        ["started_config_snapshot_id"],
    ),
}

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


def fail(message: str) -> None:
    raise RuntimeError(message)


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


def column_exists(conn: sqlite3.Connection, table: str, column: str) -> bool:
    return column in table_columns(conn, table)


def normalize_type(value: object) -> str:
    return str(value or "").strip().upper()


def normalize_default(value: object) -> str:
    if value is None:
        return ""
    return str(value).strip().strip("'\"")


def validate_column_type(
    conn: sqlite3.Connection, table: str, column: str, expected_type: str
) -> None:
    columns = table_columns(conn, table)
    if column not in columns:
        fail(f"{table}.{column} is missing")
    actual = columns[column]["type"]
    if actual != normalize_type(expected_type):
        fail(f"{table}.{column} has type {actual!r}, expected {expected_type!r}")


def validate_preflight(conn: sqlite3.Connection) -> None:
    for table, required_columns in REQUIRED_BASE_COLUMNS.items():
        if not table_exists(conn, table):
            fail(f"required table {table!r} is missing; is this a Ductile state DB?")
        columns = set(table_columns(conn, table))
        missing = sorted(required_columns - columns)
        if missing:
            fail(f"required table {table!r} is missing columns: {', '.join(missing)}")

    if table_exists(conn, "config_snapshots"):
        validate_config_snapshots_table(conn)

    for table, column, expected_type in TABLE_COLUMNS:
        if column_exists(conn, table, column):
            validate_column_type(conn, table, column, expected_type)


def ensure_column(conn: sqlite3.Connection, table: str, column: str, definition: str) -> None:
    if column_exists(conn, table, column):
        validate_column_type(conn, table, column, definition)
        return
    conn.execute(f"ALTER TABLE {table} ADD COLUMN {column} {definition}")


def validate_config_snapshots_table(conn: sqlite3.Connection) -> None:
    if not table_exists(conn, "config_snapshots"):
        fail("config_snapshots table is missing")

    columns = table_columns(conn, "config_snapshots")
    for column, expected in CONFIG_SNAPSHOT_COLUMNS.items():
        if column not in columns:
            fail(f"config_snapshots.{column} is missing")
        actual = columns[column]
        expected_type = normalize_type(expected["type"])
        if actual["type"] != expected_type:
            fail(
                f"config_snapshots.{column} has type {actual['type']!r}, "
                f"expected {expected_type!r}"
            )
        if expected.get("pk") and not actual["pk"]:
            fail(f"config_snapshots.{column} is not a primary key column")
        if expected.get("notnull") and not actual["notnull"]:
            fail(f"config_snapshots.{column} is nullable but must be NOT NULL")
        if "default" in expected:
            actual_default = normalize_default(actual["default"])
            if actual_default != expected["default"]:
                fail(
                    f"config_snapshots.{column} has default {actual_default!r}, "
                    f"expected {expected['default']!r}"
                )


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
        fail(
            f"index {index_name!r} covers {index_columns!r}, expected {columns!r}"
        )


def validate_final_schema(conn: sqlite3.Connection) -> None:
    validate_config_snapshots_table(conn)
    for table in ("job_queue", "job_log"):
        columns = table_columns(conn, table)
        for column, expected_type in JOB_SNAPSHOT_COLUMNS.items():
            if column not in columns:
                fail(f"{table}.{column} is missing after migration")
            if columns[column]["type"] != expected_type:
                fail(
                    f"{table}.{column} has type {columns[column]['type']!r}, "
                    f"expected {expected_type!r}"
                )
            if columns[column]["notnull"]:
                fail(f"{table}.{column} must remain nullable for legacy rows")

    for index_name, (table, columns) in REQUIRED_INDEXES.items():
        validate_index(conn, index_name, table, columns)


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

        validate_preflight(conn)

        with conn:
            for stmt in SCHEMA_STATEMENTS[:2]:
                conn.execute(stmt)
            for table, column, definition in TABLE_COLUMNS:
                ensure_column(conn, table, column, definition)
            for stmt in SCHEMA_STATEMENTS[2:]:
                conn.execute(stmt)

        validate_final_schema(conn)

        print(f"verified Hickey Sprint 2 config snapshot schema in {db_path}")
        return 0
    except RuntimeError as err:
        print(f"migration failed: {err}", file=sys.stderr)
        return 1
    finally:
        conn.close()


if __name__ == "__main__":
    raise SystemExit(main())
