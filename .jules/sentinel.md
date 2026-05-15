
## 2024-05-15 - [CRITICAL] Fix SQL injection in sqliteColumnExists
**Vulnerability:** A SQL injection vulnerability existed in `internal/storage/sqlite.go` where the table name was formatted directly into a `PRAGMA table_info(%s);` query using `fmt.Sprintf`.
**Learning:** SQLite `PRAGMA` statements historically did not support parameters, leading developers to use string formatting, which is unsafe when inputs are user-controlled or originate externally.
**Prevention:** Use SQLite table-valued functions (e.g., `pragma_table_info(?)`) which fully support parameterized queries to safely read schema information without string interpolation.
