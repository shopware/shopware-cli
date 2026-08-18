package tui

import (
	"context"
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

// UpdateAvailable blocks until the background update check finishes and
// reports whether a newer, installable release exists. It is set from
// cmd/root.go at startup, like AppVersion; when nil, the header skips the
// update hint entirely.
var UpdateAvailable func(context.Context) bool

// Header is the branding header row the dev dashboard, the upgrade wizard,
// and the plugin-migrate wizard render as shell chrome. It follows the Bubble
// Tea component shape (Init/Update/View) so message-driven header state can
// be added here without rewiring the hosts.
type Header struct {
	// branding is the styled "● Shopware CLI v1.0.0 · Documentation · GitHub"
	// line, rendered once at construction.
	branding       string
	width          int
	showUpdateHint bool
}

// UpdateAvailableMsg is delivered when the asynchronous update check finds a
// newer CLI version. Hosts forward it to Header.Update like any other message.
type UpdateAvailableMsg struct{}

// NewHeader creates the shared branding header.
func NewHeader() Header {
	return Header{}.withUpdateHint(false)
}

func (h Header) withUpdateHint(showUpdateHint bool) Header {
	icon := lipgloss.NewStyle().Foreground(BrandColor).Render("●")
	title := lipgloss.NewStyle().Bold(true).Foreground(TextColor).Render(appTitle)
	version := DimStyle.Render(AppVersion)

	lnkStyle := lipgloss.NewStyle().Foreground(LinkColor).Underline(true)
	docsLink := StyledLink(docsURL, "Documentation", lnkStyle)
	ghLink := StyledLink(githubURL, "GitHub", lnkStyle)

	sep := DimStyle.Render(" · ")

	updateHint := ""
	if showUpdateHint {
		updateHint = " " + lipgloss.NewStyle().Foreground(WarnColor).Render(" ⁺₊⋆ Update available ⋆₊⁺ ")
	}

	branding := icon + " " + title + " " + version + updateHint + sep + docsLink + sep + ghLink
	return Header{branding: branding, width: lipgloss.Width(branding), showUpdateHint: showUpdateHint}
}

// Init starts the update-hint wait; every host that renders the header gets
// the hint without extra wiring. Returns nil when no check is configured.
func (h Header) Init() tea.Cmd {
	return NewUpdateCheckCmd(UpdateAvailable)
}

// Update applies an update result received by the parent TUI model.
func (h Header) Update(msg tea.Msg) (Header, tea.Cmd) {
	if _, ok := msg.(UpdateAvailableMsg); ok && !h.showUpdateHint {
		return h.withUpdateHint(true), nil
	}

	return h, nil
}

// View renders the header row: the branding line right-aligned within width.
func (h Header) View(width int) string {
	fill := max(width-h.width, 0)
	return strings.Repeat(" ", fill) + h.branding
}
