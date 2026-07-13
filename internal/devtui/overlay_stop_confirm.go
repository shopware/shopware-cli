package devtui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/tui"
)

const (
	stopConfirmStop = iota
	stopConfirmQuit
	stopConfirmCancel
)

var stopConfirmLabels = []string{"Stop containers & quit", "Quit, keep running", "Cancel"}

type stopConfirmResultMsg struct {
	Stop   bool
	Cancel bool
}

type stopConfirm struct {
	selected int
}

func newStopConfirm() *stopConfirm {
	return &stopConfirm{selected: stopConfirmStop}
}

func (sc *stopConfirm) Update(msg tea.Msg) (Modal, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return sc, nil
	}

	count := len(stopConfirmLabels)
	switch key.String() {
	case keyLeft, "h":
		if sc.selected > 0 {
			sc.selected--
		}
	case keyRight, "l":
		if sc.selected < count-1 {
			sc.selected++
		}
	case keyTab:
		sc.selected = (sc.selected + 1) % count
	case keyEsc:
		return nil, emit(stopConfirmResultMsg{Cancel: true})
	case keyEnter:
		switch sc.selected {
		case stopConfirmCancel:
			return nil, emit(stopConfirmResultMsg{Cancel: true})
		default:
			return nil, emit(stopConfirmResultMsg{Stop: sc.selected == stopConfirmStop})
		}
	}
	return sc, nil
}

func (sc *stopConfirm) View(width, height int) string {
	var card strings.Builder
	warnStyle := lipgloss.NewStyle().Bold(true).Foreground(tui.ErrorColor)
	card.WriteString(warnStyle.Render("Leaving the workspace"))
	card.WriteString("\n")
	card.WriteString(tui.DimStyle.Render("Do you also want to stop the running Docker containers?\nEither way you can restart them anytime with shopware-cli project dev."))
	card.WriteString("\n\n")
	card.WriteString(renderButtonRow(stopConfirmLabels, sc.selected))

	footerHint := tui.ShortcutBar(
		tui.Shortcut{Key: "←/→", Label: "Select"},
		tui.Shortcut{Key: "enter", Label: "Confirm"},
		tui.Shortcut{Key: "esc", Label: "Cancel"},
	)
	return renderPhaseLayout(tui.RenderPhaseCard(card.String()), width, height, footerHint)
}
