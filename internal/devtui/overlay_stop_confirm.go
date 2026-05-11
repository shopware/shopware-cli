package devtui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/tui"
)

type stopConfirmResultMsg struct {
	Stop bool
}

type stopConfirm struct {
	yes bool
}

func newStopConfirm() *stopConfirm {
	return &stopConfirm{yes: true}
}

func (sc *stopConfirm) Update(msg tea.Msg) (Modal, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return sc, nil
	}

	switch key.String() {
	case keyLeft, "h":
		sc.yes = true
	case keyRight, "l":
		sc.yes = false
	case keyTab:
		sc.yes = !sc.yes
	case keyEnter:
		return nil, emit(stopConfirmResultMsg{Stop: sc.yes})
	}
	return sc, nil
}

func (sc *stopConfirm) View(width, height int) string {
	var card strings.Builder
	warnStyle := lipgloss.NewStyle().Bold(true).Foreground(tui.ErrorColor)
	card.WriteString(warnStyle.Render("Stop Docker containers?"))
	card.WriteString("\n")
	card.WriteString(tui.DimStyle.Render("Do you want to stop the running Docker containers?\nThey can be restarted with shopware-cli project dev."))
	card.WriteString("\n\n")
	card.WriteString(renderConfirmButtons("Yes, stop", "No, quit", sc.yes))

	footerHint := tui.ShortcutBar(
		tui.Shortcut{Key: "←/→", Label: "Select"},
		tui.Shortcut{Key: "enter", Label: "Confirm"},
	)
	return renderPhaseLayout(tui.RenderPhaseCard(card.String()), width, height, footerHint)
}
