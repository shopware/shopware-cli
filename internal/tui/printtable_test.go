package tui

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTableRendersTerminalAndJSONFromSameRows(t *testing.T) {
	result := NewTable(
		TableColumn{Title: "Name", JSONKey: "name"},
		TableColumn{Title: "Compatible", JSONKey: "compatible"},
	)
	result.AddRow(
		"Example",
		TableCell{Value: true, TerminalText: "Yes"},
	)

	terminal, err := result.Render(TableFormatTable)
	require.NoError(t, err)
	assert.Contains(t, terminal, "Name")
	assert.Contains(t, terminal, "Example")
	assert.Contains(t, terminal, "Yes")
	assert.NotContains(t, terminal, "true")

	jsonOutput, err := result.Render(TableFormatJSON)
	require.NoError(t, err)
	assert.JSONEq(t, `[{"name":"Example","compatible":true}]`, jsonOutput)
	assert.Equal(t, `[{"name":"Example","compatible":true}]`, jsonOutput)
}

func TestTableJSONUsesEmptyArrayForNoRows(t *testing.T) {
	result := NewTable(TableColumn{Title: "Name", JSONKey: "name"})

	output, err := result.Render(TableFormatJSON)
	require.NoError(t, err)
	assert.Equal(t, "[]", output)
}

func TestTableSupportsEstablishedJSONRowContract(t *testing.T) {
	type item struct {
		Name   string `json:"name"`
		Active bool   `json:"active"`
	}

	result := NewTable(TableColumn{Title: "Name", JSONKey: "name"})
	result.AddRowWithJSON(item{Name: "Example", Active: true}, "Example")

	terminal, err := result.Render(TableFormatTable)
	require.NoError(t, err)
	assert.Contains(t, terminal, "Example")
	assert.NotContains(t, terminal, "Active")

	jsonOutput, err := result.Render(TableFormatJSON)
	require.NoError(t, err)
	assert.Equal(t, `[{"name":"Example","active":true}]`, jsonOutput)
}

func TestTableWriteAddsTrailingNewline(t *testing.T) {
	result := NewTable(TableColumn{Title: "Name", JSONKey: "name"})
	result.AddRow("Example")

	var output bytes.Buffer
	require.NoError(t, result.Write(&output, TableFormatJSON))
	assert.Equal(t, "[{\"name\":\"Example\"}]\n", output.String())
}

func TestTableValidatesConfiguration(t *testing.T) {
	tests := []struct {
		name  string
		table *Table
		error string
	}{
		{
			name:  "no columns",
			table: NewTable(),
			error: "table must have at least one column",
		},
		{
			name:  "missing title",
			table: NewTable(TableColumn{JSONKey: "name"}),
			error: "table column 1 must have a title",
		},
		{
			name:  "missing JSON key",
			table: NewTable(TableColumn{Title: "Name"}),
			error: `table column "Name" must have a JSON key`,
		},
		{
			name: "duplicate JSON key",
			table: NewTable(
				TableColumn{Title: "Name", JSONKey: "name"},
				TableColumn{Title: "Other name", JSONKey: "name"},
			),
			error: `table JSON key "name" is duplicated`,
		},
		{
			name: "wrong row width",
			table: func() *Table {
				result := NewTable(TableColumn{Title: "Name", JSONKey: "name"})
				result.AddRow("Example", "extra")
				return result
			}(),
			error: "table row 1 has 2 cells, expected 1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.table.Render(TableFormatJSON)
			require.EqualError(t, err, test.error)
		})
	}
}

func TestParseTableFormat(t *testing.T) {
	format, err := ParseTableFormat("JSON")
	require.NoError(t, err)
	assert.Equal(t, TableFormatJSON, format)

	_, err = ParseTableFormat("yaml")
	require.EqualError(t, err, `unknown table format "yaml", allowed values: table, json`)
}
