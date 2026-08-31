package tui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
)

// TableFormat is an output format supported by Table.
type TableFormat string

const (
	TableFormatTable TableFormat = "table"
	TableFormatJSON  TableFormat = "json"
)

// ParseTableFormat validates a table output format.
func ParseTableFormat(value string) (TableFormat, error) {
	format := TableFormat(strings.ToLower(value))
	switch format {
	case TableFormatTable, TableFormatJSON:
		return format, nil
	default:
		return "", fmt.Errorf("unknown table format %q, allowed values: table, json", value)
	}
}

// TableColumn configures one terminal table column and its corresponding JSON
// object key.
type TableColumn struct {
	Title   string
	JSONKey string
}

// TableCell holds a raw value for JSON output and, optionally, a separate
// representation for terminal output. TerminalText is useful for colors,
// human-readable booleans, and other presentation that should not leak into
// machine-readable output.
type TableCell struct {
	Value        any
	TerminalText string
}

// Table contains one output-neutral table definition.
type Table struct {
	columns []TableColumn
	rows    [][]any
}

// NewTable creates a table with the given columns.
func NewTable(columns ...TableColumn) *Table {
	return &Table{columns: slices.Clone(columns)}
}

// AddRow appends a row. Row width is validated when the table is rendered.
func (t *Table) AddRow(values ...any) {
	t.rows = append(t.rows, slices.Clone(values))
}

// Render renders the table in the requested format.
func (t *Table) Render(format TableFormat) (string, error) {
	if err := t.validate(); err != nil {
		return "", err
	}

	switch format {
	case TableFormatTable:
		return t.renderTerminal(), nil
	case TableFormatJSON:
		content, err := t.renderJSON()
		if err != nil {
			return "", err
		}
		return string(content), nil
	default:
		return "", fmt.Errorf("unknown table format %q, allowed values: table, json", format)
	}
}

// Write renders the table and writes it with a trailing newline.
func (t *Table) Write(w io.Writer, format TableFormat) error {
	content, err := t.Render(format)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(w, content)
	return err
}

func (t *Table) validate() error {
	if len(t.columns) == 0 {
		return errors.New("table must have at least one column")
	}

	keys := make(map[string]struct{}, len(t.columns))
	for i, column := range t.columns {
		if column.Title == "" {
			return fmt.Errorf("table column %d must have a title", i+1)
		}
		if column.JSONKey == "" {
			return fmt.Errorf("table column %q must have a JSON key", column.Title)
		}
		if _, exists := keys[column.JSONKey]; exists {
			return fmt.Errorf("table JSON key %q is duplicated", column.JSONKey)
		}
		keys[column.JSONKey] = struct{}{}
	}

	for i, row := range t.rows {
		if len(row) != len(t.columns) {
			return fmt.Errorf("table row %d has %d cells, expected %d", i+1, len(row), len(t.columns))
		}
	}

	return nil
}

func (t *Table) renderTerminal() string {
	headers := make([]string, len(t.columns))
	for i, column := range t.columns {
		headers[i] = column.Title
	}

	cellStyle := lipgloss.NewStyle().Padding(0, 1)
	renderer := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(BorderColor)).
		StyleFunc(func(int, int) lipgloss.Style { return cellStyle }).
		Headers(headers...)

	for _, row := range t.rows {
		values := make([]string, len(row))
		for i, value := range row {
			values[i] = terminalCellText(value)
		}
		renderer.Row(values...)
	}

	return renderer.Render()
}

func (t *Table) renderJSON() ([]byte, error) {
	var output bytes.Buffer
	output.WriteByte('[')

	for rowIndex, row := range t.rows {
		if rowIndex > 0 {
			output.WriteByte(',')
		}
		output.WriteByte('{')

		for columnIndex, column := range t.columns {
			if columnIndex > 0 {
				output.WriteByte(',')
			}

			key, err := json.Marshal(column.JSONKey)
			if err != nil {
				return nil, err
			}
			value, err := json.Marshal(jsonCellValue(row[columnIndex]))
			if err != nil {
				return nil, fmt.Errorf("marshal table row %d column %q: %w", rowIndex+1, column.JSONKey, err)
			}

			output.Write(key)
			output.WriteByte(':')
			output.Write(value)
		}

		output.WriteByte('}')
	}

	output.WriteByte(']')
	return output.Bytes(), nil
}

func terminalCellText(value any) string {
	if cell, ok := value.(TableCell); ok {
		if cell.TerminalText != "" {
			return cell.TerminalText
		}
		value = cell.Value
	}
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func jsonCellValue(value any) any {
	if cell, ok := value.(TableCell); ok {
		return cell.Value
	}
	return value
}

// RenderTable renders a bordered table with a header row. Cells may be
// pre-styled; every cell gets one column of horizontal padding.
func RenderTable(headers []string, rows [][]string) string {
	columns := make([]TableColumn, len(headers))
	for i, header := range headers {
		columns[i] = TableColumn{Title: header, JSONKey: fmt.Sprintf("column%d", i+1)}
	}

	t := NewTable(columns...)
	for _, row := range rows {
		values := make([]any, len(row))
		for i, value := range row {
			values[i] = value
		}
		t.AddRow(values...)
	}

	rendered, _ := t.Render(TableFormatTable)
	return rendered
}

// PrintTable prints a bordered table to stdout — the shared shape of the
// CLI's list outputs.
func PrintTable(headers []string, rows [][]string) {
	fmt.Println(RenderTable(headers, rows))
}
