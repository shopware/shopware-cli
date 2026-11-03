package tui

import (
	"charm.land/lipgloss/v2"
)

// SectionHeadingStyle renders bold, underlined section headings in command
// output ("Project", "Next steps", "Summary", …).
var SectionHeadingStyle = lipgloss.NewStyle().Bold(true).Underline(true)

// Status glyphs for composing command output lines, e.g.
// fmt.Printf("%s Project config: %s\n", tui.CheckOK(), value).
// They are functions so rendering, and with it the terminal background
// query, happens on use rather than at package init.

// CheckOK renders a green check mark.
func CheckOK() string { return GreenText.Render("✓") }

// CheckWarn renders a yellow warning sign.
func CheckWarn() string { return SecondaryText.Render("⚠") }

// CheckFail renders a red cross.
func CheckFail() string { return RedText.Render("✗") }

// SuccessLine renders a bold green "✓ message" line.
func SuccessLine(msg string) string {
	return GreenText.Bold(true).Render("✓ " + msg)
}

// FailLine renders a bold red "✗ message" line.
func FailLine(msg string) string {
	return RedText.Bold(true).Render("✗ " + msg)
}
