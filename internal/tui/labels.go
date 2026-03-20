package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

var (
	// LabelStyle renders text in the primary text color.
	LabelStyle = lipgloss.NewStyle().Foreground(TextColor)

	// kvKeyStyle renders the key column in key-value pair rows with a fixed width.
	kvKeyStyle = lipgloss.NewStyle().Width(22).Foreground(TextColor)

	// LinkStyle renders clickable hyperlinks in a muted blue with an underline.
	LinkStyle = lipgloss.NewStyle().Foreground(LinkColor).Underline(true)

	// TitleStyle renders section headings in bold with the primary text color.
	TitleStyle = lipgloss.NewStyle().Bold(true).Foreground(TextColor)
)

// FormatLabel renders a "Label (Detail)" string where the label uses the
// primary text color and the detail is dimmed in parentheses.
func FormatLabel(label, detail string) string {
	if detail == "" {
		return LabelStyle.Render(label)
	}
	return LabelStyle.Render(label) + " " + DimStyle.Render("("+detail+")")
}

// FormatLabelDim renders a "Label (Detail)" string entirely in dimmed style.
func FormatLabelDim(label, detail string) string {
	if detail == "" {
		return DimStyle.Render(label)
	}
	return DimStyle.Render(label + " (" + detail + ")")
}

// KVRow renders a single key-value pair as a line with consistent alignment.
func KVRow(key, value string) string {
	return fmt.Sprintf("  %s%s\n", kvKeyStyle.Render(key), value)
}

// RenderStyledLink renders a URL as a clickable terminal hyperlink using LinkStyle.
func RenderStyledLink(url string) string {
	return StyledLink(url, url, LinkStyle)
}

// SectionDivider renders a full-width horizontal line in the border color.
func SectionDivider(width int) string {
	return "\n" + lipgloss.NewStyle().Foreground(BorderColor).Render(strings.Repeat("─", width)) + "\n\n"
}
