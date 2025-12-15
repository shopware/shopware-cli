package table

import (
	"fmt"
	"io"

	"github.com/charmbracelet/lipgloss"
)

// Writer is a wrapper around Charm's table that provides a similar interface to tablewriter.
type Writer struct {
	headers     []string
	rows        [][]string
	out         io.Writer
	baseStyle   lipgloss.Style
	headerStyle lipgloss.Style
}

// NewWriter creates a new table writer.
func NewWriter(out io.Writer) *Writer {
	return &Writer{
		out: out,
		baseStyle: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.NoColor{}),
		headerStyle: lipgloss.NewStyle().
			Bold(true),
	}
}

// Header sets the table headers.
func (w *Writer) SetHeader(headers []string) {
	w.headers = headers
}

// Header is an alias to SetHeader for compatibility with tablewriter.
func (w *Writer) Header(headers []string) {
	w.SetHeader(headers)
}

// Append adds a row to the table.
func (w *Writer) Append(row []string) error {
	w.rows = append(w.rows, row)
	return nil
}

// Render prints the table to the writer.
func (w *Writer) Render() error {
	// Let's build a simple ASCII table manually instead of using the Bubble Tea component
	// to ensure consistent styling across all rows

	// Calculate column widths based on content
	widths := make([]int, len(w.headers))

	// First check header lengths
	for i, h := range w.headers {
		if len(h) > widths[i] {
			widths[i] = len(h)
		}
	}

	// Then check data lengths
	for _, row := range w.rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	// Add padding
	for i := range widths {
		widths[i] += 4 // Add extra padding for better readability
	}

	// Format the header row
	headerRow := ""
	for i, h := range w.headers {
		headerRow += w.formatCell(h, widths[i])
	}

	// Write header
	_, err := fmt.Fprintln(w.out, w.headerStyle.Render(headerRow))
	if err != nil {
		return err
	}

	// Write data rows
	for _, row := range w.rows {
		dataRow := ""
		for i, cell := range row {
			if i < len(widths) {
				dataRow += w.formatCell(cell, widths[i])
			}
		}
		_, err := fmt.Fprintln(w.out, dataRow)
		if err != nil {
			return err
		}
	}

	return nil
}

// formatCell pads a cell to the given width.
func (w *Writer) formatCell(content string, width int) string {
	padded := content
	for len(padded) < width {
		padded += " "
	}
	return padded
}

// RenderTable is a convenience function to render a simple table.
func RenderTable(out io.Writer, headers []string, rows [][]string) error {
	w := NewWriter(out)
	w.SetHeader(headers)

	for _, row := range rows {
		if err := w.Append(row); err != nil {
			return err
		}
	}

	return w.Render()
}
