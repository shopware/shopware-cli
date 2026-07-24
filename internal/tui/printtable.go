package tui

import (
	"fmt"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
)

// RenderTable renders a bordered table with a header row. Cells may be
// pre-styled; every cell gets one column of horizontal padding.
func RenderTable(headers []string, rows [][]string) string {
	cellStyle := lipgloss.NewStyle().Padding(0, 1)

	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(BorderColor)).
		StyleFunc(func(int, int) lipgloss.Style { return cellStyle }).
		Headers(headers...)
	for _, row := range rows {
		t.Row(row...)
	}
	return t.Render()
}

// PrintTable prints a bordered table to stdout — the shared shape of the
// CLI's list outputs.
func PrintTable(headers []string, rows [][]string) {
	fmt.Println(RenderTable(headers, rows))
}
