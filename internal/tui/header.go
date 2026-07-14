package tui

import (
	"strings"

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

// DocsAndGitHubLinks returns the styled "Documentation · GitHub" link pair.
func DocsAndGitHubLinks() string {
	lnkStyle := lipgloss.NewStyle().Foreground(LinkColor).Underline(true)
	docsLink := StyledLink(docsURL, "Documentation", lnkStyle)
	ghLink := StyledLink(githubURL, "GitHub", lnkStyle)
	return docsLink + DimStyle.Render(" · ") + ghLink
}

// RenderSplitLine places left flush to the left edge and right flush to the
// right edge of the given width, padding the gap with spaces. If the two
// don't fit, they're joined with a single space instead of overlapping.
func RenderSplitLine(width int, left, right string) string {
	fill := width - lipgloss.Width(left) - lipgloss.Width(right)
	if fill < 1 {
		fill = 1
	}
	return left + strings.Repeat(" ", fill) + right
}

// RenderProjectHeader renders a two-line header: "Shopware CLI" and doc/GitHub
// links on the first line, project name + environment and the installed
// Shopware version on the second.
func RenderProjectHeader(width int, projectName, environment, version string) string {
	line1 := RenderSplitLine(width, BoldText.Render(appTitle), DocsAndGitHubLinks())
	line2 := RenderSplitLine(width,
		BoldText.Render(projectName)+"  "+DimStyle.Render(environment),
		BoldText.Render("Shopware "+version))
	return line1 + "\n" + line2
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
