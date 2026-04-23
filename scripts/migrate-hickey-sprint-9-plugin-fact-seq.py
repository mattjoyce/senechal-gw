#!/usr/bin/env python3
"""Add Ductile-owned plugin_facts.seq ordering to an existing Ductile DB.

This migration is intentionally additive. Existing plugin_facts rows keep a
NULL seq because their historical order was only timestamp-derived. New rows
allocated by Ductile after this migration receive monotonic seq values.
"""

import sqlite3
import sys
from pathlib import Path


SEQUENCE_NAME = "plugin_facts"

REQUIRED_PLUGIN_FACTS_COLUMNS = {
    "id": {"type": "TEXT", "pk": True},
    "plugin_name": {"type": "TEXT", "notnull": True},
    "fact_type": {"type": "TEXT", "notnull": True},
    "job_id": {"type": "TEXT", "notnull": True},
    "command": {"type": "TEXT", "notnull": True},
    "fact_json": {"type": "JSON", "notnull": True},
    "created_at": {"type": "TEXT", "notnull": True},
}

REQUIRED_SEQUENCE_COLUMNS = {
    "name": {"type": "TEXT", "pk": True},
    "value": {"type": "INTEGER", "notnull": True},
}

REQUIRED_INDEXES = {
    "plugin_facts_plugin_seq_idx": ("plugin_facts", ["plugin_name", "seq"]),
    "plugin_facts_plugin_type_seq_idx": (
        "plugin_facts",
        ["plugin_name", "fact_type", "seq"],
    ),
}

SCHEMA_STATEMENTS = [
    """
    CREATE TABLE IF NOT EXISTS storage_sequences (
      name  TEXT PRIMARY KEY,
      value INTEGER NOT NULL DEFAULT 0
    )
    """,
    """
    CREATE INDEX IF NOT EXISTS plugin_facts_plugin_seq_idx
    ON plugin_facts(plugin_name, seq)
    """,
    """
    CREATE INDEX IF NOT EXISTS plugin_facts_plugin_type_seq_idx
    ON plugin_facts(plugin_name, fact_type, seq)
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


def validate_columns(
    actual_columns: dict[str, dict[str, object]],
    expected_columns: dict[str, dict[str, object]],
    table: str,
) -> None:
    for column, expected in expected_columns.items():
        if column not in actual_columns:
            fail(f"{table}.{column} is missing")
        actual = actual_columns[column]
        expected_type = normalize_type(expected["type"])
        if actual["type"] != expected_type:
            fail(
                f"{table}.{column} has type {actual['type']!r}, "
                f"expected {expected_type!r}"
            )
        if expected.get("pk") and not actual["pk"]:
            fail(f"{table}.{column} is not a primary key column")
        if expected.get("notnull") and not actual["notnull"]:
            fail(f"{table}.{column} is nullable but must be NOT NULL")


def validate_preflight(conn: sqlite3.Connection) -> None:
    if not table_exists(conn, "plugin_facts"):
        fail(
            "plugin_facts table is missing; run "
            "scripts/migrate-hickey-sprint-7-plugin-facts.py first"
        )
    validate_columns(
        table_columns(conn, "plugin_facts"),
        REQUIRED_PLUGIN_FACTS_COLUMNS,
        "plugin_facts",
    )

    if table_exists(conn, "storage_sequences"):
        validate_columns(
            table_columns(conn, "storage_sequences"),
            REQUIRED_SEQUENCE_COLUMNS,
            "storage_sequences",
        )


def add_seq_column_if_missing(conn: sqlite3.Connection) -> None:
    columns = table_columns(conn, "plugin_facts")
    if "seq" not in columns:
        conn.execute("ALTER TABLE plugin_facts ADD COLUMN seq INTEGER")
        return
    if columns["seq"]["type"] != "INTEGER":
        fail(
            f"plugin_facts.seq has type {columns['seq']['type']!r}, "
            "expected 'INTEGER'"
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
        fail(f"index {index_name!r} covers {index_columns!r}, expected {columns!r}")


def seed_sequence(conn: sqlite3.Connection) -> None:
    max_seq = conn.execute(
        "SELECT COALESCE(MAX(seq), 0) FROM plugin_facts WHERE seq IS NOT NULL"
    ).fetchone()[0]
    conn.execute(
        """
        INSERT INTO storage_sequences(name, value)
        VALUES(?, ?)
        ON CONFLICT(name) DO UPDATE SET
          value = CASE
            WHEN storage_sequences.value < excluded.value THEN excluded.value
            ELSE storage_sequences.value
          END
        """,
        (SEQUENCE_NAME, max_seq),
    )


def validate_final_schema(conn: sqlite3.Connection) -> None:
    validate_columns(
        table_columns(conn, "storage_sequences"),
        REQUIRED_SEQUENCE_COLUMNS,
        "storage_sequences",
    )
    plugin_fact_columns = table_columns(conn, "plugin_facts")
    validate_columns(
        plugin_fact_columns,
        REQUIRED_PLUGIN_FACTS_COLUMNS,
        "plugin_facts",
    )
    if "seq" not in plugin_fact_columns:
        fail("plugin_facts.seq is missing")
    if plugin_fact_columns["seq"]["type"] != "INTEGER":
        fail(
            f"plugin_facts.seq has type {plugin_fact_columns['seq']['type']!r}, "
            "expected 'INTEGER'"
        )
    for index_name, (table, columns) in REQUIRED_INDEXES.items():
        validate_index(conn, index_name, table, columns)

    row = conn.execute(
        "SELECT value FROM storage_sequences WHERE name = ?", (SEQUENCE_NAME,)
    ).fetchone()
    if row is None:
        fail(f"storage sequence {SEQUENCE_NAME!r} is missing")
    max_seq = conn.execute(
        "SELECT COALESCE(MAX(seq), 0) FROM plugin_facts WHERE seq IS NOT NULL"
    ).fetchone()[0]
    if row[0] < max_seq:
        fail(
            f"storage sequence {SEQUENCE_NAME!r} value {row[0]} is behind "
            f"plugin_facts max seq {max_seq}"
        )


def main() -> int:
    if len(sys.argv) != 2:
        print(
            "usage: scripts/migrate-hickey-sprint-9-plugin-fact-seq.py <path-to-sqlite-db>",
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
            conn.execute(SCHEMA_STATEMENTS[0])
            add_seq_column_if_missing(conn)
            for stmt in SCHEMA_STATEMENTS[1:]:
                conn.execute(stmt)
            seed_sequence(conn)

        validate_final_schema(conn)

        print(f"verified Hickey Sprint 9 plugin fact seq schema in {db_path}")
        return 0
    except RuntimeError as err:
        print(f"migration failed: {err}", file=sys.stderr)
        return 1
    finally:
        conn.close()


if __name__ == "__main__":
    raise SystemExit(main())
