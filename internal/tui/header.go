package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
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

// Header is the branding header row the dev dashboard, the upgrade wizard,
// and the plugin-migrate wizard render as shell chrome. It follows the Bubble
// Tea component shape (Init/Update/View) so message-driven header state can
// be added here without rewiring the hosts.
type Header struct {
	// branding is the styled "● Shopware CLI v1.0.0 · Documentation · GitHub"
	// line, rendered once at construction; width is its visual width in
	// terminal columns.
	branding string
	width    int
}

// NewHeader creates the shared branding header.
func NewHeader() Header {
	icon := lipgloss.NewStyle().Foreground(BrandColor).Render("●")
	title := lipgloss.NewStyle().Bold(true).Foreground(TextColor).Render(appTitle)
	version := DimStyle.Render(AppVersion)

	lnkStyle := lipgloss.NewStyle().Foreground(LinkColor).Underline(true)
	docsLink := StyledLink(docsURL, "Documentation", lnkStyle)
	ghLink := StyledLink(githubURL, "GitHub", lnkStyle)

	sep := DimStyle.Render(" · ")

	branding := icon + " " + title + " " + version + sep + docsLink + sep + ghLink
	return Header{branding: branding, width: lipgloss.Width(branding)}
}

// Init implements the component contract; the header has no startup work.
func (h Header) Init() tea.Cmd {
	return nil
}

// Update implements the component contract; the header holds no
// message-driven state yet.
func (h Header) Update(tea.Msg) (Header, tea.Cmd) {
	return h, nil
}

// View renders the header row: the branding line right-aligned within width.
func (h Header) View(width int) string {
	fill := max(width-h.width, 0)
	return strings.Repeat(" ", fill) + h.branding
}
