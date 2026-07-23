package tui

import (
	"charm.land/lipgloss/v2"
)

// SectionHeadingStyle renders bold, underlined section headings in command
// output ("Project", "Next steps", "Summary", …).
var SectionHeadingStyle = lipgloss.NewStyle().Bold(true).Underline(true)

// Status glyphs for composing command output lines, e.g.
// fmt.Printf("%s Project config: %s\n", tui.CheckOK, value).
var (
	CheckOK   = GreenText.Render("✓")
	CheckWarn = SecondaryText.Render("⚠")
	CheckFail = RedText.Render("✗")
)

// SuccessLine renders a bold green "✓ message" line.
func SuccessLine(msg string) string {
	return GreenText.Bold(true).Render("✓ " + msg)
}

// FailLine renders a bold red "✗ message" line.
func FailLine(msg string) string {
	return RedText.Bold(true).Render("✗ " + msg)
}
