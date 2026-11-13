package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
	return path
}

func openMemoryDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestLoadCSV(t *testing.T) {
	path := writeTempFile(t, "sales.csv", "id,region,amount\n1,North,150\n2,South,\n")

	table, err := loadFile(path)
	if err != nil {
		t.Fatalf("loadFile: %v", err)
	}

	if table.name != "sales" {
		t.Errorf("table name = %q, want %q", table.name, "sales")
	}
	if len(table.columns) != 3 {
		t.Fatalf("columns = %d, want 3", len(table.columns))
	}
	if len(table.rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(table.rows))
	}
	if table.rows[0][1] != "North" {
		t.Errorf("first region = %v, want North", table.rows[0][1])
	}
	if table.rows[1][2] != nil {
		t.Errorf("empty cell = %v, want nil", table.rows[1][2])
	}
}

func TestLoadTSV(t *testing.T) {
	path := writeTempFile(t, "data.tsv", "a\tb\n1\t2\n")

	table, err := loadFile(path)
	if err != nil {
		t.Fatalf("loadFile: %v", err)
	}
	if len(table.columns) != 2 || table.columns[1] != "b" {
		t.Fatalf("columns = %v, want [a b]", table.columns)
	}
}

func TestLoadJSONUnionOfKeys(t *testing.T) {
	path := writeTempFile(t, "users.json",
		`[{"id":1,"name":"Ana"},{"id":2,"meta":{"plan":"pro"}}]`)

	table, err := loadFile(path)
	if err != nil {
		t.Fatalf("loadFile: %v", err)
	}

	want := []string{"id", "meta", "name"}
	if len(table.columns) != len(want) {
		t.Fatalf("columns = %v, want %v", table.columns, want)
	}
	for i, column := range want {
		if table.columns[i] != column {
			t.Errorf("column %d = %q, want %q", i, table.columns[i], column)
		}
	}

	if len(table.rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(table.rows))
	}
	if table.rows[1][1] != `{"plan":"pro"}` {
		t.Errorf("nested object = %v, want its JSON text", table.rows[1][1])
	}
	if table.rows[0][2] != "Ana" {
		t.Errorf("name = %v, want Ana", table.rows[0][2])
	}
}

func TestLoadJSONLines(t *testing.T) {
	path := writeTempFile(t, "events.jsonl", "{\"id\":1}\n\n{\"id\":2}\n")

	table, err := loadFile(path)
	if err != nil {
		t.Fatalf("loadFile: %v", err)
	}
	if len(table.rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(table.rows))
	}
}

func TestLoadUnsupportedExtension(t *testing.T) {
	path := writeTempFile(t, "notes.txt", "hello")

	if _, err := loadFile(path); err == nil {
		t.Fatal("expected an error for an unsupported extension")
	}
}

func TestTableName(t *testing.T) {
	cases := map[string]string{
		"/tmp/sales.csv":      "sales",
		"/tmp/Sales Data.csv": "sales_data",
		"/tmp/2024-01.csv":    "t_2024_01",
		"/tmp/---.csv":        "data",
		"/tmp/My_File.v2.tsv": "my_file_v2",
		"/tmp/REPORT.JSONL":   "report",
	}

	for path, want := range cases {
		if got := tableName(path); got != want {
			t.Errorf("tableName(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestInferType(t *testing.T) {
	cases := []struct {
		name string
		rows [][]any
		want string
	}{
		{
			name: "integers",
			rows: [][]any{{int64(1)}, {int64(2)}},
			want: "INTEGER",
		},
		{
			name: "numeric text",
			rows: [][]any{{"1"}, {"2"}},
			want: "INTEGER",
		},
		{
			name: "decimals",
			rows: [][]any{{int64(1)}, {1.5}},
			want: "REAL",
		},
		{
			name: "mixed text",
			rows: [][]any{{"1"}, {"two"}},
			want: "TEXT",
		},
		{
			name: "only empty values",
			rows: [][]any{{nil}, {nil}},
			want: "TEXT",
		},
		{
			name: "no rows",
			rows: nil,
			want: "TEXT",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := inferType(tc.rows, 0); got != tc.want {
				t.Errorf("inferType = %q, want %q", got, tc.want)
			}
		})
	}
}
