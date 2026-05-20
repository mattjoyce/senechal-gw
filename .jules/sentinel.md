## 2024-05-14 - Fix SQL injection in sqliteColumnExists
**Vulnerability:** SQL injection vulnerability in `internal/storage/sqlite.go` due to formatting `PRAGMA table_info(%s)` with user input.
**Learning:** `PRAGMA` statements in SQLite cannot be parameterized directly. This can lead to SQL injection vulnerabilities when dynamically building PRAGMA statements with user input.
**Prevention:** Use SQLite table-valued functions like `pragma_table_info(?)` which allow for parameterization when querying schema metadata dynamically.
