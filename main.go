package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"

	_ "modernc.org/sqlite"
)

const usageText = `file-sql: run SQL against CSV, TSV, JSON and JSONL files.

Usage:
  file-sql [flags] <file> [file...]

Every file becomes a table named after the file (sales.csv -> sales).
Flags must come before the files.

Examples:
  file-sql -q "SELECT region, SUM(amount) FROM sales GROUP BY region" sales.csv
  file-sql -format csv -q "SELECT * FROM users LIMIT 10" users.json
`

func main() {
	queryFlag := flag.String("q", "", "SQL to run against the loaded files")
	sqlFlag := flag.String("f", "", "path to a file containing the SQL to run")
	formatFlag := flag.String("format", "table", "output format: table, csv or json")
	outFlag := flag.String("o", "", "write the result to this file instead of stdout")

	flag.Usage = func() {
		fmt.Fprint(flag.CommandLine.Output(), usageText)
		fmt.Fprintln(flag.CommandLine.Output(), "Flags:")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}

	query := strings.TrimSpace(*queryFlag)
	if *sqlFlag != "" {
		data, err := os.ReadFile(*sqlFlag)
		if err != nil {
			fail(err)
		}
		query = strings.TrimSpace(string(data))
	}
	if query == "" {
		fail(fmt.Errorf("no SQL given: use -q or -f"))
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		fail(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	for _, path := range flag.Args() {
		t, err := loadFile(path)
		if err != nil {
			fail(err)
		}
		if err := createTable(db, t); err != nil {
			fail(fmt.Errorf("%s: %w", path, err))
		}
	}

	result, err := runQuery(db, query)
	if err != nil {
		fail(err)
	}

	out := os.Stdout
	if *outFlag != "" {
		file, err := os.Create(*outFlag)
		if err != nil {
			fail(err)
		}
		defer file.Close()
		out = file
	}

	if err := writeResult(out, result, *formatFlag); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
