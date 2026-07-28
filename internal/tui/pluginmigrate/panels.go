package pluginmigrate

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	migrate "github.com/shopware/shopware-cli/internal/shop/pluginmigrate"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/tui/app"
)

// --- Welcome -----------------------------------------------------------------

func (m *Model) updateWelcome(msg tea.Msg) (app.Content, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}

	ks := app.KeyString(key)
	m.welcomeYes = tui.ConfirmNav(m.welcomeYes, ks)
	switch ks {
	case "q", "esc":
		return m, tea.Quit
	case "enter":
		if !m.scanDone {
			return m, nil
		}
		if !m.welcomeYes || len(m.scan) == 0 {
			return m, tea.Quit
		}
		m.panel = panelToken
		m.tokenFocus = 0
		m.tokenInput.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

func (m *Model) viewWelcome() string {
	var b strings.Builder

	switch {
	case !m.scanDone:
		b.WriteString(tui.BoldStyle.Render("Scanning custom/ for extensions…"))
	case len(m.scan) == 0:
		b.WriteString(tui.BoldStyle.Render("All extensions are already managed through Composer."))
		b.WriteString("\n\n")
		b.WriteString(tui.DimStyle.Render("Nothing to do here — `project upgrade` will be happy."))
		b.WriteString("\n\n")
		b.WriteString(tui.NewButtonRow(tui.ButtonRowOptions{Labels: []string{"Close"}, Active: 0}).Render())
		return tui.RenderPhaseCardCowsay("Everything is Composer-managed. Nice!", b.String())
	default:
		b.WriteString(tui.BoldStyle.Render(fmt.Sprintf("Found %d extensions in custom/ that Composer does not manage:", len(m.scan))))
		b.WriteString("\n\n")
		for _, ext := range m.scan {
			version := ext.Version
			if version == "" {
				version = "unknown version"
			}
			b.WriteString("  • " + tui.LabelStyle.Render(ext.Name) + " " + tui.DimStyle.Render(version) + "\n")
		}
		b.WriteString("\n")
		b.WriteString(tui.DimStyle.Render("Store plugins get required from packages.shopware.com; local plugins"))
		b.WriteString("\n")
		b.WriteString(tui.DimStyle.Render("become Composer path repositories. Files change only after a review."))
		b.WriteString("\n\n")
		b.WriteString(tui.ConfirmButtons("Let's fix this", "Cancel", m.welcomeYes))
	}

	return tui.RenderPhaseCardCowsay("Let's get your plugins under Composer control!", b.String())
}

// --- Token -------------------------------------------------------------------

func (m *Model) updateToken(msg tea.Msg) (app.Content, tea.Cmd) {
	if m.tokenBusy {
		return m, nil
	}
	if paste, ok := msg.(tea.PasteMsg); ok && m.tokenFocus == 0 {
		var cmd tea.Cmd
		m.tokenInput, cmd = m.tokenInput.Update(paste)
		m.tokenErr = ""
		return m, cmd
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}

	switch app.KeyString(key) {
	case "esc":
		m.panel = panelWelcome
		m.tokenInput.Blur()
		return m, nil
	case "up", "down", tui.KeyTab:
		m.tokenFocus = 1 - m.tokenFocus
		if m.tokenFocus == 0 {
			m.tokenInput.Focus()
			return m, textinput.Blink
		}
		m.tokenInput.Blur()
		return m, nil
	case "enter":
		if m.tokenFocus == 1 {
			// Continue without a token: Packagist and configured repositories
			// are still checked, everything else becomes a path repository.
			m.tokenBusy = true
			m.tokenErr = ""
			return m, fetchAvailabilityCmd(m.migrator, "", m.scan)
		}
		token := strings.TrimSpace(m.tokenInput.Value())
		if token == "" {
			m.tokenErr = "Enter a token, or continue without one below."
			return m, nil
		}
		m.tokenBusy = true
		m.tokenErr = ""
		return m, fetchAvailabilityCmd(m.migrator, token, m.scan)
	}

	if m.tokenFocus == 0 {
		var cmd tea.Cmd
		m.tokenInput, cmd = m.tokenInput.Update(key)
		m.tokenErr = ""
		return m, cmd
	}
	return m, nil
}

func (m *Model) handleAvailability(msg availabilityMsg) (app.Content, tea.Cmd) {
	m.tokenBusy = false
	if msg.err != nil {
		m.tokenErr = "Could not fetch the Store packages: " + msg.err.Error()
		return m, nil
	}
	return m.beginReview(msg.token, msg.avail)
}

func (m *Model) beginReview(token string, avail migrate.Availability) (app.Content, tea.Cmd) {
	m.token = token
	m.plan = migrate.BuildPlan(m.scan, avail)
	m.panel = panelReview
	m.reviewApply = true
	m.tokenInput.Blur()
	return m, nil
}

func (m *Model) viewToken() (title, status, body string) {
	title = "Connect the Shopware Store (optional)"
	if m.tokenBusy {
		status = m.statusStrip(tui.VariantInfo, "CHECKING", "Looking your plugins up in the Composer repositories…")
	} else if m.tokenErr != "" {
		status = m.statusStrip(tui.VariantError, "ERROR", m.tokenErr)
	}

	var b strings.Builder
	b.WriteString(tui.BoldStyle.Render("Shopware Packagist token"))
	b.WriteString("\n\n")
	b.WriteString(tui.LabelStyle.Render("With a token, plugins bought in the Shopware Store are required from"))
	b.WriteString("\n")
	b.WriteString(tui.LabelStyle.Render("packages.shopware.com and keep receiving updates through Composer."))
	b.WriteString("\n\n")
	b.WriteString(tui.DimStyle.Render("Get yours at ") +
		tui.StyledLink("https://account.shopware.com", "account.shopware.com", tui.LinkStyle) +
		tui.DimStyle.Render(" → Merchant area → Shops → Packagist."))
	b.WriteString("\n\n")
	b.WriteString(m.tokenInput.View())
	b.WriteString("\n\n")
	skipActive := -1
	if m.tokenFocus == 1 {
		skipActive = 0
	}
	b.WriteString(tui.NewButtonRow(tui.ButtonRowOptions{
		Labels: []string{"Continue without token — manage everything locally"},
		Active: skipActive,
	}).Render())
	b.WriteString("\n\n")
	b.WriteString(tui.DimStyle.Render("Without a token, plugins published on Packagist are still required from"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("there; everything else becomes a path repository — fully Composer-managed"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("either way, just without Store updates."))

	return title, status, b.String()
}

// --- Review ------------------------------------------------------------------

func (m *Model) updateReview(msg tea.Msg) (app.Content, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}

	ks := app.KeyString(key)
	m.reviewApply = tui.ConfirmNav(m.reviewApply, ks)
	switch ks {
	case "esc":
		m.panel = panelToken
		m.tokenFocus = 0
		m.tokenInput.Focus()
		return m, textinput.Blink
	case "q":
		return m, tea.Quit
	case "enter":
		if !m.plan.Actionable() {
			return m, tea.Quit
		}
		if !m.reviewApply {
			return m, tea.Quit
		}
		return m.beginRun()
	}
	return m, nil
}

func (m *Model) viewReview() (title, status, body string) {
	title = "Review the migration plan"

	if !m.plan.Actionable() {
		status = m.statusStrip(tui.VariantError, "BLOCKED", "None of the extensions can be migrated automatically.")
	} else {
		status = m.statusStrip(tui.VariantInfo, "REVIEW", m.summaryCounts()+". Nothing changes before Apply.")
	}

	var b strings.Builder
	rows := make([][]string, 0, len(m.plan.Extensions))
	for _, ext := range m.plan.Extensions {
		version := ext.Version
		if version == "" {
			version = "-"
		}
		rows = append(rows, []string{ext.Name, version, ext.Kind.Label()})
	}
	b.WriteString(tui.RenderTable([]string{"Extension", "Version", "Action"}, rows))
	b.WriteString("\n")

	b.WriteString(tui.BoldStyle.Render("Configuration changes"))
	b.WriteString("\n")
	if m.plan.AddStoreRepository {
		b.WriteString(tui.DimStyle.Render("  • add Composer repository "+migrate.StoreRepositoryURL) + "\n")
	}
	for _, path := range m.plan.PathRepositories() {
		b.WriteString(tui.DimStyle.Render("  • add path repository "+path) + "\n")
	}
	if m.token != "" {
		b.WriteString(tui.DimStyle.Render("  • store the Packagist token in auth.json") + "\n")
	}
	if len(m.plan.RemoveDirs()) > 0 {
		b.WriteString(tui.DimStyle.Render(fmt.Sprintf("  • remove %d migrated directories after the require succeeded", len(m.plan.RemoveDirs()))) + "\n")
	}
	b.WriteString("\n")

	if m.plan.Actionable() {
		b.WriteString(tui.ConfirmButtons("Apply", "Cancel", m.reviewApply))
	} else {
		b.WriteString(tui.NewButtonRow(tui.ButtonRowOptions{Labels: []string{"Close"}, Active: 0}).Render())
	}

	return title, status, b.String()
}

// --- Run ---------------------------------------------------------------------

type runState struct {
	states    map[migrate.StepID]migrate.StepState
	log       []string
	events    <-chan migrate.StepEvent
	finished  bool
	succeeded bool
	err       error
	cancel    context.CancelFunc
}

const runLogKeep = 500

func (m *Model) beginRun() (app.Content, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	m.panel = panelRun
	m.run = runState{
		states: make(map[migrate.StepID]migrate.StepState),
		cancel: cancel,
		events: m.migrator.Run(ctx, m.plan, m.token),
	}

	return m, readRunEventCmd(m.run.events)
}

func (m *Model) updateRun(msg tea.Msg) (app.Content, tea.Cmd) {
	switch msg := msg.(type) {
	case runEventMsg:
		ev := migrate.StepEvent(msg)
		switch {
		case ev.Line != "":
			m.run.log = tui.AppendTail(m.run.log, runLogKeep, ev.Line)
		case ev.Step == migrate.StepFinished:
			m.run.finished = true
			m.run.succeeded = ev.State == migrate.StateOK
			m.run.err = ev.Err
		default:
			m.run.states[ev.Step] = ev.State
		}
		return m, readRunEventCmd(m.run.events)

	case runClosedMsg:
		if m.run.cancel != nil {
			m.run.cancel()
		}
		m.panel = panelDone
		m.done = doneState{succeeded: m.run.succeeded, err: m.run.err}
		return m, nil
	}
	return m, nil
}

func (m *Model) viewRun() (title, status, body string) {
	title = "Migrating plugins to Composer"
	if !m.run.finished {
		status = m.statusStrip(tui.VariantInfo, "RUNNING", "Applying the migration — ctrl+c cancels and rolls back.")
	}

	items := make([]tui.StepItem, 0, len(migrate.RunSteps))
	for _, step := range migrate.RunSteps {
		item := tui.StepItem{Label: step.Label()}
		switch m.run.states[step] {
		case migrate.StateRunning:
			item.State = tui.StepStateActive
		case migrate.StateOK, migrate.StateWarn:
			item.State = tui.StepStateDone
		case migrate.StateFail:
			item.Label = failStyle.Render(step.Label())
			item.State = tui.StepStateDone
		case migrate.StatePending:
			item.State = tui.StepStatePending
		}
		items = append(items, item)
	}

	var b strings.Builder
	b.WriteString(tui.NewStepList(tui.StepListOptions{Steps: items}).Render())
	b.WriteString("\n")

	visible := m.frameHeight() - len(migrate.RunSteps) - 8
	if visible < 3 {
		visible = 3
	}
	for _, line := range tui.TailLines(m.run.log, visible) {
		b.WriteString(tui.DimStyle.Render(tui.Truncate(line, m.bodyWidth())))
		b.WriteString("\n")
	}

	return title, status, b.String()
}

func (m *Model) updateDone(msg tea.Msg) (app.Content, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch app.KeyString(key) {
	case "enter", "q", "esc":
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) viewDone() string {
	var b strings.Builder

	if m.done.succeeded {
		b.WriteString(tui.BoldStyle.Render("All extensions are now managed through Composer."))
		b.WriteString("\n\n")
		if n := m.plan.Count(migrate.ActionStoreRequire); n > 0 {
			b.WriteString(okStyle.Render("✓") + tui.LabelStyle.Render(fmt.Sprintf(" %d plugins now come from the Shopware Store", n)) + "\n")
		}
		if n := m.plan.Count(migrate.ActionPathRepository); n > 0 {
			b.WriteString(okStyle.Render("✓") + tui.LabelStyle.Render(fmt.Sprintf(" %d local plugins are path repositories", n)) + "\n")
		}
		b.WriteString("\n")
		b.WriteString(tui.BoldStyle.Render("Next steps"))
		b.WriteString("\n")
		b.WriteString(tui.DimStyle.Render("  1. Verify the shop still works") + "\n")
		b.WriteString(tui.DimStyle.Render("  2. Commit composer.json, composer.lock, and auth.json") + "\n")
		b.WriteString(tui.DimStyle.Render("  3. `project upgrade` can now resolve every extension") + "\n")
		b.WriteString("\n")
		b.WriteString(tui.NewButtonRow(tui.ButtonRowOptions{Labels: []string{"Close"}, Active: 0}).Render())
		return tui.RenderPhaseCardCowsay("All plugins are Composer-managed now!", b.String())
	}

	b.WriteString(failStyle.Render("The migration did not complete."))
	b.WriteString("\n")
	b.WriteString(tui.LabelStyle.Render("composer.json, composer.lock, and auth.json were restored."))
	b.WriteString("\n\n")
	if m.done.err != nil {
		b.WriteString(failStyle.Render(tui.Truncate(m.done.err.Error(), tui.PhaseCardWidth-10)))
		b.WriteString("\n\n")
	}
	b.WriteString(tui.NewButtonRow(tui.ButtonRowOptions{Labels: []string{"Close"}, Active: 0}).Render())
	return tui.RenderPhaseCard(b.String())
}
