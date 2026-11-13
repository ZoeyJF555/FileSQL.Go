# file-sql

Run SQL against the files on your disk. `file-sql` loads **CSV, TSV, JSON and
JSONL** files into an in-memory SQLite database and runs a SQL query over them.

Written in Go with **no CGO** (it uses the pure-Go `modernc.org/sqlite`
driver), so it builds to a single static binary.

```
file-sql -q "SELECT region, SUM(amount) FROM sales GROUP BY region" sales.csv
```

---

## Table of contents

- [Features](#features)
- [Install](#install)
- [Usage](#usage)
- [Examples](#examples)
- [Supported input](#supported-input)
- [Type inference](#type-inference)
- [Testing](#testing)
- [Project layout](#project-layout)
- [Architecture](#architecture)
- [Limitations](#limitations)
- [License](#license)

---

## Features

- Query **CSV, TSV, JSON and JSONL** files with SQL, no import step.
- Every file becomes a table named after the file (`sales.csv` -> `sales`).
- Join across files in a single query.
- Automatic **column type inference** (`INTEGER`, `REAL`, `TEXT`); empty cells
  become `NULL`.
- Nested JSON objects and arrays are kept as JSON text and can be read back with
  SQLite's `json_extract`.
- Three output formats: aligned **table** (default), **CSV** and **JSON**.
- Write results to `stdout` or a file.
- Pure Go, no CGO, single static binary.

## Install

With a Go toolchain:

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
`users.json` is `users`. Flags must come before the file list.

| Flag | Meaning |
| ---- | ------- |
| `-q` | SQL query to run |
| `-f` | read the SQL from a file instead |
| `-format` | `table` (default), `csv` or `json` |
| `-o` | write the result to a file instead of stdout |

If neither `-q` nor `-f` is given, `file-sql` exits with an error. Running with
no files prints the usage text.

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

For JSON, the table columns are the **union of all keys** across the objects,
sorted alphabetically so the layout is stable between runs. Missing keys become
`NULL`. Empty values become `NULL`. Nested objects and arrays are stored as JSON
text.

## Type inference

A column's type is inferred from its data before the table is created:

| Condition | Column type |
| --------- | ----------- |
| Every non-empty value is a whole number | `INTEGER` |
| Every non-empty value is a number (integer or decimal) | `REAL` |
| Otherwise (or no values at all) | `TEXT` |

In JSON, booleans are stored as `1` / `0` and `null` becomes `NULL`.

## Testing

The project has unit tests for the loader, type inference and query/output
pipeline, plus a smoke test that runs in CI.

```bash
go vet ./...
go test ./... -v
```

Test files:

- `load_test.go` — loading CSV/TSV/JSON/JSONL, JSON key union, table-name
  derivation, and `inferType` (integers, numeric text, decimals, mixed text,
  empty and no rows).
- `query_test.go` — end-to-end CSV query, joins across files, table/CSV/JSON
  writers, unknown-format errors, query errors, and NULL handling.

### Continuous integration

`.github/workflows/ci.yml` runs on every push and pull request:

1. `go vet ./...` — static analysis.
2. `go test ./... -v` — the full test suite.
3. `go build -o file-sql .` — build the binary.
4. A smoke test that queries a generated CSV and checks the CSV output header.

## Project layout

```
main.go        flags, wiring, exit codes
load.go        file readers, type inference, table creation
output.go      query execution and table/CSV/JSON writers
load_test.go   loader and type-inference tests
query_test.go  query pipeline and writer tests
go.mod         module definition
```

## Architecture

`file-sql` is a small pipeline: **load -> create tables -> query -> format**.

### Loading (`load.go`)

- `loadFile` dispatches on the file extension to `loadDelimited` (CSV/TSV) or
  `loadJSON` (JSON/JSONL).
- `loadDelimited` reads the header row and treats every following record as a
  row; empty cells become `nil` (SQL `NULL`).
- `loadJSON` reads an array of objects (or one object per line), computes the
  union of keys, and converts each value with `jsonValue`.
- `createTable` infers each column type, creates the table, and inserts all rows
  inside a single transaction using a prepared statement.
- `tableName` derives a safe, lowercase table name from the file name (e.g.
  `Sales Data.csv` -> `sales_data`, `2024-01.csv` -> `t_2024_01`).

### Query and output (`output.go`)

- `runQuery` executes the SQL and collects the rows into a `result`.
- `writeResult` dispatches on `-format` to `writeTable`, `writeCSV` or
  `writeJSON`.

### Entry point (`main.go`)

- Parses flags, loads the SQL from `-q` or `-f`, opens an in-memory SQLite
  database, loads each file into a table, runs the query, and writes the result
  to `stdout` or the `-o` file.

## Limitations

- Everything runs in memory, so files larger than available RAM will not fit.
- One query per run.
- Only SQLite's SQL dialect is available.

## License

No license file is currently included in this repository.
