package upgradetui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/tui/app"
)

type introState struct {
	button int // 0 = Begin upgrade, 1 = Cancel
}

func newIntroState() introState {
	return introState{}
}

func (m *Model) updateIntro(msg tea.Msg) (app.Content, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}

	switch app.KeyString(key) {
	case "left", "h":
		m.intro.button = 0
	case "right", "l", "tab":
		m.intro.button = 1 - m.intro.button
	case "enter":
		if m.intro.button == 0 {
			return m.beginChecks()
		}
		return m, tea.Quit
	case "q", "esc":
		return m, tea.Quit
	}
	return m, nil
}

// beginChecks moves to the readiness panel and starts the checks.
func (m *Model) beginChecks() (app.Content, tea.Cmd) {
	m.panel = panelCheck
	m.check = newCheckState()
	m.check.loading = true
	return m, runChecksCmd(m.upgrader)
}

func (m *Model) viewIntro() (title, status, body string) {
	title = "Upgrade Shopware to a newer version"

	var left strings.Builder
	left.WriteString(tui.LabelStyle.Render("This wizard will guide you through a ") + tui.BoldStyle.Render("local"))
	left.WriteString("\n")
	left.WriteString(tui.LabelStyle.Render("Shopware upgrade:"))
	left.WriteString("\n\n")
	steps := []string{
		"Check project readiness",
		"Choose the Shopware version to install",
		"Check Composer-managed extensions",
		"Update project files",
		"Run Deployment Helper",
	}
	for i, step := range steps {
		left.WriteString(tui.DimStyle.Render("  "+string(rune('1'+i))+".  ") + tui.LabelStyle.Render(step) + "\n")
	}
	left.WriteString("\n\n")
	left.WriteString(tui.BoldStyle.Render("Before files change"))
	left.WriteString("\n")
	left.WriteString(tui.DimStyle.Render("You will review the upgrade plan before the"))
	left.WriteString("\n")
	left.WriteString(tui.DimStyle.Render("wizard applies it ") + tui.BoldStyle.Render("locally") + tui.DimStyle.Render("."))
	left.WriteString("\n\n")
	left.WriteString(tui.DimStyle.Render("We do not check custom project extensions."))

	var right strings.Builder
	right.WriteString(tui.LabelStyle.Render("After the wizard finishes:"))
	right.WriteString("\n\n")
	right.WriteString(tui.DimStyle.Render("  • ") + tui.LabelStyle.Render("test the shop ") + tui.BoldStyle.Render("locally") + "\n")
	right.WriteString(tui.DimStyle.Render("  • ") + tui.LabelStyle.Render("commit the changed files") + "\n")
	right.WriteString(tui.DimStyle.Render("  • ") + tui.LabelStyle.Render("deploy through your normal process") + "\n")
	right.WriteString("\n\n")
	right.WriteString(userActionStyle.Render("User action"))
	right.WriteString("\n\n")
	right.WriteString(tui.BoldStyle.Render("Begin upgrade") + tui.LabelStyle.Render(" starts the guided checks"))
	right.WriteString("\n")
	right.WriteString(tui.LabelStyle.Render("and version selection."))
	right.WriteString("\n\n")
	right.WriteString(tui.DimStyle.Render("No project files change yet."))
	right.WriteString("\n\n")
	right.WriteString(m.buttonWrap(m.rightColumnWidth(m.bodyWidth()*11/20),
		[]string{"Begin upgrade", "Cancel"}, m.intro.button))

	body = m.twoColumn(m.bodyWidth()*11/20, left.String(), right.String())
	return title, "", body
}
