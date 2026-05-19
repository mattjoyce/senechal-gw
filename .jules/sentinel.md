
## 2024-05-19 - SQL Injection in PRAGMA table_info
**Vulnerability:** SQL injection via unparameterized string concatenation in PRAGMA table_info queries.
**Learning:** SQLite's PRAGMA statements cannot be directly parameterized. However, modern SQLite provides table-valued functions like `pragma_table_info(?)` which allow for safe parameterized queries. The "notnull" column must be quoted since it's a reserved keyword.
**Prevention:** Always use table-valued functions (`pragma_*()`) instead of PRAGMA statements when checking schema metadata to ensure user input can be safely parameterized.
