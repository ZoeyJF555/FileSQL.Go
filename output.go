package main

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type result struct {
	columns []string
	rows    [][]any
}

func runQuery(db *sql.DB, query string) (*result, error) {
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	values := make([]any, len(columns))
	pointers := make([]any, len(columns))
	for i := range values {
		pointers[i] = &values[i]
	}

	var collected [][]any
	for rows.Next() {
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}
		row := make([]any, len(columns))
		copy(row, values)
		collected = append(collected, row)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &result{columns: columns, rows: collected}, nil
}

func writeResult(w io.Writer, r *result, format string) error {
	switch strings.ToLower(format) {
	case "table":
		return writeTable(w, r)
	case "csv":
		return writeCSV(w, r)
	case "json":
		return writeJSON(w, r)
	default:
		return fmt.Errorf("unknown format %q: use table, csv or json", format)
	}
}

func writeTable(w io.Writer, r *result) error {
	widths := make([]int, len(r.columns))
	for i, column := range r.columns {
		widths[i] = len(column)
	}

	cells := make([][]string, len(r.rows))
	for rowIndex, row := range r.rows {
		cells[rowIndex] = make([]string, len(r.columns))
		for columnIndex := range r.columns {
			text := displayValue(row[columnIndex])
			cells[rowIndex][columnIndex] = text
			if len(text) > widths[columnIndex] {
				widths[columnIndex] = len(text)
			}
		}
	}

	writeRow := func(values []string) error {
		var builder strings.Builder
		for i, value := range values {
			builder.WriteString(value)
			if i < len(values)-1 {
				builder.WriteString(strings.Repeat(" ", widths[i]-len(value)+2))
			}
		}
		_, err := fmt.Fprintln(w, strings.TrimRight(builder.String(), " "))
		return err
	}

	if err := writeRow(r.columns); err != nil {
		return err
	}

	separators := make([]string, len(widths))
	for i, width := range widths {
		separators[i] = strings.Repeat("-", width)
	}
	if err := writeRow(separators); err != nil {
		return err
	}

	for _, row := range cells {
		if err := writeRow(row); err != nil {
			return err
		}
	}

	_, err := fmt.Fprintf(w, "\n%d row(s)\n", len(r.rows))
	return err
}

func writeCSV(w io.Writer, r *result) error {
	writer := csv.NewWriter(w)
	if err := writer.Write(r.columns); err != nil {
		return err
	}

	for _, row := range r.rows {
		record := make([]string, len(r.columns))
		for i := range r.columns {
			record[i] = displayValue(row[i])
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	writer.Flush()
	return writer.Error()
}

func writeJSON(w io.Writer, r *result) error {
	records := make([]map[string]any, len(r.rows))
	for rowIndex, row := range r.rows {
		record := make(map[string]any, len(r.columns))
		for columnIndex, column := range r.columns {
			record[column] = jsonValueOut(row[columnIndex])
		}
		records[rowIndex] = record
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(records)
}

func jsonValueOut(value any) any {
	switch typed := value.(type) {
	case []byte:
		return string(typed)
	default:
		return value
	}
}

func displayValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case []byte:
		return string(typed)
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", typed), "0"), ".")
	default:
		return fmt.Sprint(typed)
	}
}
