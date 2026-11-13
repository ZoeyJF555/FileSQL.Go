package main

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// table is the in-memory representation of a file before it is written to
// SQLite.
type table struct {
	name    string
	columns []string
	rows    [][]any
}

func loadFile(path string) (*table, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".csv":
		return loadDelimited(path, ',')
	case ".tsv":
		return loadDelimited(path, '\t')
	case ".json":
		return loadJSON(path, false)
	case ".jsonl", ".ndjson":
		return loadJSON(path, true)
	default:
		return nil, fmt.Errorf(
			"%s: unsupported file type, use .csv, .tsv, .json or .jsonl", path)
	}
}

func loadDelimited(path string, separator rune) (*table, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = separator
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("%s: cannot read the header row: %w", path, err)
	}

	rows := make([][]any, 0, 1024)
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}

		row := make([]any, len(header))
		for i := range header {
			if i < len(record) && record[i] != "" {
				row[i] = record[i]
			}
		}
		rows = append(rows, row)
	}

	return &table{name: tableName(path), columns: header, rows: rows}, nil
}

func loadJSON(path string, lines bool) (*table, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	objects := make([]map[string]json.RawMessage, 0, 1024)
	if lines {
		for number, line := range strings.Split(string(content), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var object map[string]json.RawMessage
			if err := json.Unmarshal([]byte(line), &object); err != nil {
				return nil, fmt.Errorf("%s:%d: %w", path, number+1, err)
			}
			objects = append(objects, object)
		}
	} else {
		if err := json.Unmarshal(content, &objects); err != nil {
			return nil, fmt.Errorf(
				"%s: expected a JSON array of objects: %w", path, err)
		}
	}

	columns := collectKeys(objects)
	rows := make([][]any, 0, len(objects))
	for _, object := range objects {
		row := make([]any, len(columns))
		for i, column := range columns {
			if raw, ok := object[column]; ok {
				row[i] = jsonValue(raw)
			}
		}
		rows = append(rows, row)
	}

	return &table{name: tableName(path), columns: columns, rows: rows}, nil
}

// collectKeys returns the union of every key in the objects, sorted so the
// table layout is stable between runs.
func collectKeys(objects []map[string]json.RawMessage) []string {
	seen := make(map[string]struct{}, len(objects))
	for _, object := range objects {
		for key := range object {
			seen[key] = struct{}{}
		}
	}

	columns := make([]string, 0, len(seen))
	for key := range seen {
		columns = append(columns, key)
	}
	sort.Strings(columns)
	return columns
}

func jsonValue(raw json.RawMessage) any {
	trimmed := strings.TrimSpace(string(raw))
	switch {
	case trimmed == "null":
		return nil
	case trimmed == "true":
		return int64(1)
	case trimmed == "false":
		return int64(0)
	case strings.HasPrefix(trimmed, "\""):
		var text string
		if err := json.Unmarshal(raw, &text); err == nil {
			return text
		}
		return trimmed
	case strings.HasPrefix(trimmed, "{"), strings.HasPrefix(trimmed, "["):
		return trimmed
	default:
		if number, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			return number
		}
		if number, err := strconv.ParseFloat(trimmed, 64); err == nil {
			return number
		}
		return trimmed
	}
}

// createTable writes the table into SQLite, choosing a column type from the
// data so that sorting and aggregation behave as expected.
func createTable(db *sql.DB, t *table) error {
	if len(t.columns) == 0 {
		return fmt.Errorf("%s: no columns found", t.name)
	}

	columnTypes := make([]string, len(t.columns))
	definitions := make([]string, len(t.columns))
	for i, column := range t.columns {
		columnTypes[i] = inferType(t.rows, i)
		definitions[i] = quoteIdentifier(column) + " " + columnTypes[i]
	}

	query := fmt.Sprintf("CREATE TABLE %s (%s)",
		quoteIdentifier(t.name), strings.Join(definitions, ", "))
	if _, err := db.Exec(query); err != nil {
		return err
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(t.columns)), ", ")
	insert := fmt.Sprintf("INSERT INTO %s VALUES (%s)", quoteIdentifier(t.name), placeholders)

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	statement, err := tx.Prepare(insert)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer statement.Close()

	for _, row := range t.rows {
		values := make([]any, len(t.columns))
		for i := range t.columns {
			if i < len(row) {
				values[i] = convertValue(row[i], columnTypes[i])
			}
		}
		if _, err := statement.Exec(values...); err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

// inferType returns INTEGER when every non-empty value is a whole number,
// REAL when they are all numbers, and TEXT otherwise.
func inferType(rows [][]any, column int) string {
	seen := false
	allIntegers := true
	allNumbers := true

	for _, row := range rows {
		if column >= len(row) || row[column] == nil {
			continue
		}
		seen = true

		switch value := row[column].(type) {
		case int64:
		case float64:
			allIntegers = false
		case string:
			if _, err := strconv.ParseInt(value, 10, 64); err != nil {
				allIntegers = false
			}
			if _, err := strconv.ParseFloat(value, 64); err != nil {
				allNumbers = false
			}
		default:
			allIntegers = false
			allNumbers = false
		}

		if !allNumbers && !allIntegers {
			return "TEXT"
		}
	}

	switch {
	case !seen:
		return "TEXT"
	case allNumbers && allIntegers:
		return "INTEGER"
	case allNumbers:
		return "REAL"
	default:
		return "TEXT"
	}
}

// convertValue turns the raw value into something SQLite stores with the type
// chosen for the column.
func convertValue(value any, columnType string) any {
	if value == nil {
		return nil
	}

	text, isText := value.(string)
	if columnType == "TEXT" {
		if isText {
			return text
		}
		return fmt.Sprint(value)
	}

	if isText {
		text = strings.TrimSpace(text)
		if text == "" {
			return nil
		}
		if columnType == "INTEGER" {
			if number, err := strconv.ParseInt(text, 10, 64); err == nil {
				return number
			}
		}
		if number, err := strconv.ParseFloat(text, 64); err == nil {
			return number
		}
		return text
	}

	return value
}

func quoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// tableName derives a table name from the file name, keeping it usable
// without quoting.
func tableName(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

	var builder strings.Builder
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteRune('_')
		}
	}

	name := strings.Trim(builder.String(), "_")
	if name == "" {
		return "data"
	}
	if name[0] >= '0' && name[0] <= '9' {
		return "t_" + name
	}
	return strings.ToLower(name)
}
