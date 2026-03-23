package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// SelectOption describes a single option in a select list.
type SelectOption struct {
	Label  string
	Detail string
}

// RenderSelectList renders a titled option list with a ● selector on the active item.
func RenderSelectList(title, description string, options []SelectOption, cursor int) string {
	var s strings.Builder

	selectorStyle := lipgloss.NewStyle().Foreground(BrandColor)
	selectedStyle := lipgloss.NewStyle().Foreground(BrandColor)

	s.WriteString(TitleStyle.Render(title))
	s.WriteString("\n")
	if description != "" {
		s.WriteString(lipgloss.NewStyle().Foreground(MutedColor).Render(description))
		s.WriteString("\n")
	}
	s.WriteString("\n")

	for i, opt := range options {
		detail := ""
		if opt.Detail != "" {
			detail = " " + DimStyle.Render("("+opt.Detail+")")
		}
		if i == cursor {
			s.WriteString(selectorStyle.Render("● ") + selectedStyle.Render(opt.Label) + detail)
		} else {
			s.WriteString("  " + FormatLabel(opt.Label, opt.Detail))
		}
		if i < len(options)-1 {
			s.WriteString("\n")
		}
	}

	return s.String()
}
