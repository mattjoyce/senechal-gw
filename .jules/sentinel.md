## 2025-05-12 - Fix SQL injection in SQLite schema check
**Vulnerability:** SQL injection vulnerability in `sqliteColumnExists` where `fmt.Sprintf` was used to interpolate table names into a `PRAGMA table_info()` query.
**Learning:** SQLite's `PRAGMA` statements cannot be parameterized directly. However, they are also exposed as table-valued functions (e.g. `pragma_table_info(?)`), which *do* support parameterization.
**Prevention:** Use parameterized table-valued functions (like `pragma_table_info(?)`) instead of string interpolation for schema introspection to prevent SQL injection.
