package upgradetui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/shop/upgrade"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/tui/app"
)

// extensionList is the full-list overlay (panel 3a): every extension with its
// version transition, scrollable, with a copy-as-text action.
type extensionList struct {
	results     []upgrade.ExtensionResult
	targetLabel string
	cursor      int
	scroll      int
	pageSize    int
	copied      bool
}

func newExtensionList(results []upgrade.ExtensionResult, targetLabel string) extensionList {
	return extensionList{results: results, targetLabel: targetLabel, pageSize: 12}
}

func (l *extensionList) Init() tea.Cmd { return nil }

func (l *extensionList) ID() string { return "extension-list" }

func (l *extensionList) Update(msg tea.Msg) (app.Overlay, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return l, nil
	}

	switch app.KeyString(key) {
	case "esc", "q":
		return nil, app.Emit(overlayClosedMsg{})
	case "up", "k":
		if l.cursor > 0 {
			l.cursor--
		}
	case "down", "j":
		if l.cursor < len(l.results)-1 {
			l.cursor++
		}
	case "enter":
		if l.cursor < len(l.results) {
			detail := newExtensionDetail(l.results[l.cursor], l.targetLabel)
			return &detail, nil
		}
	case "y":
		l.copied = true
		return l, tea.SetClipboard(l.copyText())
	}

	if l.cursor < l.scroll {
		l.scroll = l.cursor
	}
	if l.cursor >= l.scroll+l.pageSize {
		l.scroll = l.cursor - l.pageSize + 1
	}
	return l, nil
}

// copyText renders the list as plain text for sharing with a team or vendor.
func (l *extensionList) copyText() string {
	var b strings.Builder
	for _, r := range l.results {
		b.WriteString(r.Extension.Name + "  " + versionTransition(r) + "  " + r.Status.Label() + "\n")
	}
	return b.String()
}

func (l *extensionList) View(width, height int) string {
	modal := tui.NewModal(tui.ModalOptions{MaxWidth: 70, AreaWidth: width, AreaHeight: height})
	innerWidth := modal.ContentWidth()

	var b strings.Builder
	b.WriteString(tui.SpreadRow(innerWidth,
		lipgloss.NewStyle().Bold(true).Foreground(tui.BrandColor).Render("Extensions"),
		tui.DimStyle.Render(l.targetLabel),
	))
	b.WriteString("\n\n")
	b.WriteString(tui.DimStyle.Render("These are all checked extensions with their planned version transition."))
	b.WriteString("\n\n")

	end := min(l.scroll+l.pageSize, len(l.results))

	var rows []string
	for i := l.scroll; i < end; i++ {
		rows = append(rows, extensionQueueRow(l.results[i], i == l.cursor, 26, 22))
	}
	table := strings.Join(rows, "\n")
	if bar := tui.NewScrollbar(tui.ScrollbarOptions{Total: len(l.results), Visible: l.pageSize, Offset: l.scroll, Height: max(len(rows), 3)}).Render(); bar != "" {
		table = tui.JoinColumns(table, bar, 1)
	}
	b.WriteString(table)
	b.WriteString("\n\n")

	copyLabel := "Copy list"
	if l.copied {
		copyLabel = "Copied!"
	}
	b.WriteString(tui.ShortcutBarFit(innerWidth,
		tui.Shortcut{Key: "↑/↓", Label: "Scroll"},
		tui.Shortcut{Key: "enter", Label: "View details"},
		tui.Shortcut{Key: "y", Label: copyLabel},
		tui.Shortcut{Key: "esc", Label: "Back"},
	))

	return modal.Render(b.String())
}
