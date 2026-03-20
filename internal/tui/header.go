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

// RenderHeader renders the full CLI header box containing the logo, branding
// line, description, and documentation/bug-report links.
func RenderHeader() string {
	arrow := lipgloss.NewStyle().Foreground(BrandColor).Render("●")
	title := lipgloss.NewStyle().Bold(true).Foreground(TextColor).Render(appTitle)
	version := DimStyle.Render(AppVersion)
	titleLine := arrow + " " + title + " " + version

	desc := DimStyle.Render("Manage your Shopware projects, extensions, and local development environments.")

	help := DimStyle.Render("Need help? Visit the ")
	lnkStyle := lipgloss.NewStyle().Foreground(LinkColor).Underline(true)
	link := StyledLink(docsURL, "Shopware Documentation", lnkStyle)
	dot := DimStyle.Render(".")

	bugText := DimStyle.Render("Found a bug? Create an issue on ")
	bugLink := StyledLink(githubURL, "GitHub", lnkStyle)
	bugDot := DimStyle.Render(".")

	content := titleLine + "\n" + desc + "\n" + help + link + dot + "\n" + bugText + bugLink + bugDot

	w := TerminalWidth()
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(BorderColor).
		Padding(1, 6).
		Width(w).
		Render(content)
}
