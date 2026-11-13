package main

import (
	"bytes"
	"strings"
	"testing"
)

func loadCSVTable(t *testing.T, content string) *table {
	t.Helper()

	loaded, err := loadFile(writeTempFile(t, "sales.csv", content))
	if err != nil {
		t.Fatalf("loadFile: %v", err)
	}
	return loaded
}

func TestEndToEndCSVQuery(t *testing.T) {
	db := openMemoryDB(t)

	if err := createTable(db, loadCSVTable(t,
		"region,amount\nNorth,150\nNorth,210\nSouth,90\n")); err != nil {
		t.Fatalf("createTable: %v", err)
	}

	result, err := runQuery(db,
		"SELECT region, SUM(amount) AS total FROM sales GROUP BY region ORDER BY total DESC")
	if err != nil {
		t.Fatalf("runQuery: %v", err)
	}

	var out bytes.Buffer
	if err := writeResult(&out, result, "csv"); err != nil {
		t.Fatalf("writeResult: %v", err)
	}

	want := "region,total\nNorth,360\nSouth,90\n"
	if out.String() != want {
		t.Errorf("csv output =\n%q\nwant\n%q", out.String(), want)
	}
}

func TestJoinAcrossFiles(t *testing.T) {
	db := openMemoryDB(t)

	if err := createTable(db, loadCSVTable(t, "id,amount\n1,150\n2,90\n")); err != nil {
		t.Fatalf("createTable sales: %v", err)
	}

	users, err := loadFile(writeTempFile(t, "users.json",
		`[{"id":1,"name":"Ana"},{"id":2,"name":"Luis"}]`))
	if err != nil {
		t.Fatalf("loadFile users: %v", err)
	}
	if err := createTable(db, users); err != nil {
		t.Fatalf("createTable users: %v", err)
	}

	result, err := runQuery(db,
		"SELECT name FROM users JOIN sales ON sales.id = users.id WHERE amount > 100")
	if err != nil {
		t.Fatalf("runQuery: %v", err)
	}

	if len(result.rows) != 1 || result.rows[0][0] != "Ana" {
		t.Fatalf("rows = %v, want a single row with Ana", result.rows)
	}
}

func TestWriteJSON(t *testing.T) {
	db := openMemoryDB(t)
	if err := createTable(db, loadCSVTable(t, "region,amount\nNorth,150\n")); err != nil {
		t.Fatalf("createTable: %v", err)
	}

	result, err := runQuery(db, "SELECT region, amount FROM sales")
	if err != nil {
		t.Fatalf("runQuery: %v", err)
	}

	var out bytes.Buffer
	if err := writeResult(&out, result, "json"); err != nil {
		t.Fatalf("writeResult: %v", err)
	}

	for _, expected := range []string{`"region": "North"`, `"amount": 150`} {
		if !strings.Contains(out.String(), expected) {
			t.Errorf("json output %q does not contain %q", out.String(), expected)
		}
	}
}

func TestWriteTable(t *testing.T) {
	db := openMemoryDB(t)
	if err := createTable(db, loadCSVTable(t, "region,amount\nNorth,150\n")); err != nil {
		t.Fatalf("createTable: %v", err)
	}

	result, err := runQuery(db, "SELECT region FROM sales")
	if err != nil {
		t.Fatalf("runQuery: %v", err)
	}

	var out bytes.Buffer
	if err := writeResult(&out, result, "table"); err != nil {
		t.Fatalf("writeResult: %v", err)
	}

	text := out.String()
	for _, expected := range []string{"region", "North", "1 row(s)"} {
		if !strings.Contains(text, expected) {
			t.Errorf("table output %q does not contain %q", text, expected)
		}
	}
}

func TestWriteUnknownFormat(t *testing.T) {
	result := &result{columns: []string{"a"}, rows: [][]any{{int64(1)}}}

	var out bytes.Buffer
	if err := writeResult(&out, result, "yaml"); err == nil {
		t.Fatal("expected an error for an unknown format")
	}
}

func TestQueryError(t *testing.T) {
	db := openMemoryDB(t)

	if _, err := runQuery(db, "SELECT * FROM missing_table"); err == nil {
		t.Fatal("expected an error for a missing table")
	}
}

func TestNullsAndNullableColumns(t *testing.T) {
	db := openMemoryDB(t)
	if err := createTable(db, loadCSVTable(t, "a,b\n1,\n,2\n")); err != nil {
		t.Fatalf("createTable: %v", err)
	}

	result, err := runQuery(db, "SELECT COUNT(b) AS filled FROM sales")
	if err != nil {
		t.Fatalf("runQuery: %v", err)
	}

	if len(result.rows) != 1 || displayValue(result.rows[0][0]) != "1" {
		t.Fatalf("counted non-null values = %v, want 1", result.rows)
	}
}
