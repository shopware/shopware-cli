package devtui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/tui"
)

type paletteCommand struct {
	Label    string
	Shortcut string
	ID       string
}

var paletteCommands = []paletteCommand{
	{Label: "Open Storefront", ID: "open-shop"},
	{Label: "Open Admin", ID: "open-admin"},
	{Label: "Clear Cache", ID: "cache-clear"},
	{Label: "Toggle Logs Tab", Shortcut: "2", ID: "tab-logs"},
	{Label: "Toggle General Tab", Shortcut: "1", ID: "tab-general"},
	{Label: "Quit", Shortcut: "ctrl+c", ID: "quit"},
}

type commandPalette struct {
	filter   textinput.Model
	cursor   int
	filtered []int // indices into paletteCommands
}

func newCommandPalette() commandPalette {
	ti := textinput.New()
	ti.Prompt = lipgloss.NewStyle().Foreground(tui.BrandColor).Render("> ")
	ti.Placeholder = "Type to filter"
	ti.CharLimit = 64
	ti.Focus()

	cp := commandPalette{
		filter: ti,
	}
	cp.applyFilter()
	return cp
}

func (cp *commandPalette) applyFilter() {
	query := strings.ToLower(cp.filter.Value())
	cp.filtered = nil
	for i, cmd := range paletteCommands {
		if query == "" || strings.Contains(strings.ToLower(cmd.Label), query) {
			cp.filtered = append(cp.filtered, i)
		}
	}
	if cp.cursor >= len(cp.filtered) {
		cp.cursor = max(len(cp.filtered)-1, 0)
	}
}

func (cp *commandPalette) moveUp() {
	if cp.cursor > 0 {
		cp.cursor--
	}
}

func (cp *commandPalette) moveDown() {
	if cp.cursor < len(cp.filtered)-1 {
		cp.cursor++
	}
}

func (cp commandPalette) selectedID() string {
	if len(cp.filtered) == 0 {
		return ""
	}
	return paletteCommands[cp.filtered[cp.cursor]].ID
}

func (cp commandPalette) view(width, height int) string {
	paletteWidth := min(width-4, 70)
	innerWidth := paletteWidth - 6 // border(2) + padding(4)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(tui.BrandColor)

	var b strings.Builder

	b.WriteString(titleStyle.Render("Commands"))
	b.WriteString("\n\n")
	b.WriteString(cp.filter.View())
	b.WriteString("\n\n")

	selectedStyle := lipgloss.NewStyle().
		Foreground(tui.TextColor).
		Background(tui.BrandColor).
		Width(innerWidth)

	normalStyle := lipgloss.NewStyle().
		Foreground(tui.TextColor).
		Width(innerWidth)

	shortcutStyle := lipgloss.NewStyle().
		Foreground(tui.MutedColor)

	selectedShortcutStyle := lipgloss.NewStyle().
		Foreground(tui.TextColor).
		Background(tui.BrandColor)

	for i, idx := range cp.filtered {
		cmd := paletteCommands[idx]
		rowStyle, scStyle := normalStyle, shortcutStyle
		if i == cp.cursor {
			rowStyle, scStyle = selectedStyle, selectedShortcutStyle
		}

		if cmd.Shortcut != "" {
			sc := scStyle.Render(cmd.Shortcut)
			gap := max(innerWidth-lipgloss.Width(cmd.Label)-lipgloss.Width(cmd.Shortcut), 1)
			b.WriteString(rowStyle.Render(cmd.Label + strings.Repeat(" ", gap) + sc))
		} else {
			b.WriteString(rowStyle.Render(cmd.Label))
		}
		b.WriteString("\n")
	}

	if len(cp.filtered) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(tui.MutedColor).Render("No matching commands"))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(tui.ShortcutBar(
		tui.Shortcut{Key: "↑/↓", Label: "Choose"},
		tui.Shortcut{Key: "enter", Label: "Confirm"},
		tui.Shortcut{Key: "esc", Label: "Cancel"},
	))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.BrandColor).
		Padding(1, 2).
		Width(paletteWidth)

	modal := box.Render(b.String())

	return lipgloss.Place(
		width,
		height,
		lipgloss.Center,
		lipgloss.Center,
		modal,
	)
}
