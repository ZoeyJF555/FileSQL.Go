# file-sql

Run SQL against the files on your disk. `file-sql` loads CSV, TSV, JSON and
JSONL files into an in-memory SQLite database and runs a query over them.

Written in Go, no CGO, single binary.

## Install

```bash
go install github.com/ZoeyJF555/filesql-go@latest
```

Or build from a checkout:

```bash
go build -o file-sql .
```

## Usage

```
file-sql [flags] <file> [file...]
```

Every file becomes a table named after the file: `sales.csv` is `sales`,
`users.json` is `users`.

| Flag | Meaning |
| ---- | ------- |
| `-q` | SQL query to run |
| `-f` | read the SQL from a file instead |
| `-format` | `table` (default), `csv` or `json` |
| `-o` | write the result to a file instead of stdout |

Flags must come before the file list.

## Examples

```bash
# Group and aggregate across a CSV
file-sql -q "SELECT region, SUM(amount) AS total
             FROM sales GROUP BY region ORDER BY total DESC" sales.csv

# Join a CSV and a JSON file
file-sql -q "SELECT u.name, s.amount
             FROM users u JOIN sales s ON s.user_id = u.id" users.json sales.csv

# Export the result as CSV
file-sql -format csv -o report.csv -q "SELECT * FROM sales WHERE amount > 100" sales.csv

# Nested JSON objects are kept as JSON text and can be read back
file-sql -q "SELECT json_extract(meta, '$.plan') AS plan, COUNT(*) FROM users GROUP BY plan" users.json
```

## Supported input

| Extension | Read as |
| --------- | ------- |
| `.csv` | comma-separated, first row is the header |
| `.tsv` | tab-separated, first row is the header |
| `.json` | array of objects |
| `.jsonl`, `.ndjson` | one object per line |

Column types are inferred: a column becomes `INTEGER` when every value is a
whole number, `REAL` when every value is a number, and `TEXT` otherwise. Empty
values become `NULL`. Nested objects and arrays are stored as JSON text.

## Limitations

- Everything runs in memory, so files larger than available RAM will not fit.
- One query per run.
- Only SQLite's SQL dialect is available.

## Layout

```
main.go     flags, wiring, exit codes
load.go     file readers, type inference, table creation
output.go   query execution and table/CSV/JSON writers
```
