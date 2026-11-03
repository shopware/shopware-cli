package tui

import (
	"charm.land/lipgloss/v2"
)

// SectionHeadingStyle renders bold, underlined section headings in command
// output ("Project", "Next steps", "Summary", …).
var SectionHeadingStyle = lipgloss.NewStyle().Bold(true).Underline(true)

// CheckOK renders a green check mark for prefixing command output lines.
// The status glyphs are functions so the terminal background is detected
// when they are rendered, not at package init.
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
