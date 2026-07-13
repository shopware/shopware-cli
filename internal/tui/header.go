package tui

import (
	"charm.land/lipgloss/v2"
)

const (
	appTitle  = "Shopware CLI"
	docsURL   = "https://developer.shopware.com/docs/products/cli/"
	githubURL = "https://github.com/shopware/shopware-cli"
)

// AppVersion is the CLI version displayed in headers and branding lines.
// It is set from cmd/root.go at startup.
var AppVersion = "dev"

// BrandingLine returns the fully styled branding string:
// "● Shopware CLI v1.0.0 · Documentation · GitHub"
func BrandingLine() string {
	icon := lipgloss.NewStyle().Foreground(BrandColor).Render("●")
	title := lipgloss.NewStyle().Bold(true).Foreground(TextColor).Render(appTitle)
	version := DimStyle.Render(AppVersion)

	lnkStyle := lipgloss.NewStyle().Foreground(LinkColor).Underline(true)
	docsLink := StyledLink(docsURL, "Documentation", lnkStyle)
	ghLink := StyledLink(githubURL, "GitHub", lnkStyle)

	sep := DimStyle.Render(" · ")

	return icon + " " + title + " " + version + sep + docsLink + sep + ghLink
}

// BrandingLineWidth returns the visual width of the branding line in terminal columns.
func BrandingLineWidth() int {
	return lipgloss.Width("●") + 1 +
		lipgloss.Width(appTitle) + 1 +
		lipgloss.Width(AppVersion) +
		lipgloss.Width(" · ") +
		lipgloss.Width("Documentation") +
		lipgloss.Width(" · ") +
		lipgloss.Width("GitHub")
}
