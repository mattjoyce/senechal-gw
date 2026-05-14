## 2026-05-14 - Fix SQL Injection in sqliteColumnExists
**Vulnerability:** A SQL injection vulnerability existed in `sqliteColumnExists` in `internal/storage/sqlite.go` where user input could be injected into a `PRAGMA table_info` query via `fmt.Sprintf` string interpolation.
**Learning:** SQLite's `PRAGMA` statements do not support parameterization directly. Consequently, using `fmt.Sprintf` for constructing PRAGMA queries is vulnerable to SQL injection if the table name is derived from user input or is otherwise untrusted.
**Prevention:** Use table-valued functions (like `pragma_table_info(?)`) which allow parameterized inputs and safely encapsulate variables instead of using string interpolation for `PRAGMA` queries.
