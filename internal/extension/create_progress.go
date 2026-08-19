package extension

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"

	"github.com/shopware/shopware-cli/internal/tui"
)

type createProgressTickMsg struct{}

var placeholderCreateSteps = []string{
	"Placeholder: creating extension directory",
	"Placeholder: writing composer.json",
	"Placeholder: generating plugin class",
	"Placeholder: scaffolding selected files",
}

func (m *createForm) beginProgress() tea.Cmd {
	m.values(&m.result)
	m.phase = phaseProgress
	m.step = 0
	m.logs = nil
	m.progress = progress.New(
		progress.WithColors(tui.BrandColor),
		progress.WithWidth(48),
	)

	return func() tea.Msg { return createProgressTickMsg{} }
}

func (m *createForm) updateProgress(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case progress.FrameMsg:
		var cmd tea.Cmd
		m.progress, cmd = m.progress.Update(msg)
		return m, cmd
	case createProgressTickMsg:
		if m.step >= len(placeholderCreateSteps) {
			m.phase = phaseDone
			return m, nil
		}
		m.logs = append(m.logs, placeholderCreateSteps[m.step])
		m.step++
		pct := float64(m.step) / float64(len(placeholderCreateSteps))
		return m, tea.Batch(
			m.progress.SetPercent(pct),
			tea.Tick(400*time.Millisecond, func(time.Time) tea.Msg {
				return createProgressTickMsg{}
			}),
		)
	case tea.KeyPressMsg:
		switch tui.KeyString(msg) {
		case tui.KeyCtrlC, tui.KeyEsc:
			m.cancelled = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *createForm) updateDone(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch tui.KeyString(key) {
	case tui.KeyEnter, tui.KeyEsc, tui.KeyCtrlC, "q":
		return m, tea.Quit
	}
	return m, nil
}

func (m *createForm) renderProgress() string {
	var b strings.Builder

	b.WriteString(tui.SectionTitleStyle.Render("Creating extension"))
	b.WriteString("\n\n")
	b.WriteString(m.progress.View())
	b.WriteString("\n\n")
	b.WriteString(tui.BoldStyle.Render("Logs"))
	b.WriteString("\n")
	for _, line := range m.logs {
		b.WriteString(tui.DimStyle.Render("  " + line))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(tui.ShortcutBar(
		tui.Shortcut{Key: "esc", Label: "Cancel"},
	))
	b.WriteString("\n")

	return b.String()
}

func (m *createForm) renderDone() string {
	var b strings.Builder

	b.WriteString(tui.Checkmark)
	b.WriteString(" ")
	b.WriteString(tui.SectionTitleStyle.Render("Extension created"))
	b.WriteString("\n\n")
	b.WriteString(tui.NewStatusStrip(tui.StatusStripOptions{
		Variant: tui.VariantSuccess,
		Label:   "DONE",
		Message: "Placeholder: the extension was created successfully.",
	}).Render())
	b.WriteString("\n\n")
	b.WriteString(tui.KVRow("Name", tui.LabelStyle.Render(m.result.Name)))
	b.WriteString(tui.KVRow("Namespace", tui.LabelStyle.Render(m.result.Namespace)))
	b.WriteString("\n")
	b.WriteString(tui.BoldStyle.Render("Included"))
	b.WriteString("\n")
	b.WriteString(m.renderSummaryOptions())
	b.WriteString("\n")
	b.WriteString(tui.ShortcutBar(
		tui.Shortcut{Key: "enter", Label: "Close"},
	))
	b.WriteString("\n")

	return b.String()
}

func (m *createForm) renderSummaryOptions() string {
	var selected []string
	for _, opt := range m.options {
		if opt.checked {
			selected = append(selected, "  "+tui.Checkmark+" "+opt.label)
		}
	}
	if len(selected) == 0 {
		return tui.DimStyle.Render("  No scaffolding options selected.") + "\n"
	}
	return strings.Join(selected, "\n") + "\n"
}
