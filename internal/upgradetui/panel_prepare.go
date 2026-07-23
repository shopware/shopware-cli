package upgradetui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/shop/upgrade"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/tui/app"
)

// prepareState backs panel 3: system preparation checks and the extension
// compatibility queue. Nothing here modifies project files.
type prepareState struct {
	// gen identifies the preparation run these results belong to. Async
	// results from a superseded run (user backed out and re-entered with a
	// different target) carry an older gen and are dropped.
	gen int

	envRunning   *bool
	envErr       error
	packagist    *bool
	resolve      *upgrade.ResolveResult
	resolveErr   error
	results      []upgrade.ExtensionResult
	compatDone   bool
	phpDone      bool
	phpReq       string
	phpInstalled string
	// reportRequested marks that the failure report write was kicked off;
	// reportPath or reportErr carry its outcome.
	reportRequested bool
	reportPath      string
	reportErr       error

	cursor int
	scroll int
	// queueRow remembers the selected queue row while the Continue button is
	// focused, so moving back left restores it.
	queueRow int
}

// beginPrepare enters panel 3 and starts all preparation checks in parallel.
func (m *Model) beginPrepare() (app.Content, tea.Cmd) {
	m.panel = panelPrepare
	m.prepareGen++
	m.prepare = prepareState{gen: m.prepareGen}
	target := m.check.target()
	return m, tea.Batch(
		envStatusCmd(m.opts.Executor, m.prepareGen),
		packagistCmd(m.upgrader, m.prepareGen),
		resolveCmd(m.upgrader, target.Version.String(), m.prepareGen),
		compatCmd(m.upgrader, m.check.readiness.CurrentVersion, target.Version, m.check.readiness.Extensions, m.prepareGen),
		phpInfoCmd(m.upgrader, target.Version, m.prepareGen),
	)
}

// blockers counts extensions that prevent the upgrade from starting.
func (s prepareState) blockers() int {
	n := 0
	for _, r := range s.results {
		if r.Status.BlocksUpgrade() {
			n++
		}
	}
	return n
}

// flagged counts extensions that need the user's attention — blocking,
// deprecated, or manual-review findings. While any exist the queue stays
// visible so they are not hidden behind other failure output.
func (s prepareState) flagged() int {
	n := 0
	for _, r := range s.results {
		if r.Status.NeedsAttention() {
			n++
		}
	}
	return n
}

// loading reports whether any preparation check is still running.
func (s prepareState) loading() bool {
	return s.envRunning == nil || s.packagist == nil || (s.resolve == nil && s.resolveErr == nil) || !s.compatDone
}

// applyResolved overwrites the metadata-derived target versions with the
// exact releases the composer dry run picked, once both checks finished.
func (s *prepareState) applyResolved() {
	if s.resolve == nil || !s.compatDone {
		return
	}
	upgrade.ApplyResolvedVersions(s.results, *s.resolve)
}

// ready reports whether the wizard may continue to the review panel. The
// Composer dry-run is the authoritative gate: extensions are passed with a
// "*" constraint, so the solver decides whether a compatible set exists —
// flagged extensions alone do not block.
func (s prepareState) ready() bool {
	if s.loading() {
		return false
	}
	return s.resolve != nil && s.resolve.OK
}

func (m *Model) updatePrepare(msg tea.Msg) (app.Content, tea.Cmd) {
	switch msg := msg.(type) {
	case envStatusMsg:
		if msg.gen != m.prepare.gen {
			return m, nil
		}
		running := msg.running
		m.prepare.envRunning = &running
		m.prepare.envErr = msg.err
		return m, m.maybeWriteFailureReport()

	case packagistMsg:
		if msg.gen != m.prepare.gen {
			return m, nil
		}
		reachable := msg.reachable
		m.prepare.packagist = &reachable
		return m, m.maybeWriteFailureReport()

	case resolveDoneMsg:
		if msg.gen != m.prepare.gen {
			return m, nil
		}
		if msg.err != nil {
			m.prepare.resolveErr = msg.err
		} else {
			result := msg.result
			m.prepare.resolve = &result
		}
		m.prepare.applyResolved()
		return m, m.maybeWriteFailureReport()

	case reportWrittenMsg:
		m.prepare.reportPath = msg.path
		m.prepare.reportErr = msg.err
		if msg.err != nil {
			m.prepare.reportPath = ""
		}
		return m, nil

	case compatDoneMsg:
		if msg.gen != m.prepare.gen {
			return m, nil
		}
		m.prepare.results = msg.results
		m.prepare.compatDone = true
		m.prepare.cursor = 0
		m.prepare.scroll = 0
		m.prepare.applyResolved()
		return m, m.maybeWriteFailureReport()

	case phpInfoMsg:
		if msg.gen != m.prepare.gen {
			return m, nil
		}
		m.prepare.phpDone = true
		m.prepare.phpReq = msg.requirement
		m.prepare.phpInstalled = msg.installed
		return m, m.maybeWriteFailureReport()

	case tea.KeyPressMsg:
		return m.updatePrepareKeys(msg)
	}
	return m, nil
}

// maybeWriteFailureReport writes the failure report once every preparation
// result has arrived and the resolution failed — the review panel's export is
// unreachable in that state, and exporting earlier would snapshot incomplete
// extension and PHP data.
func (m *Model) maybeWriteFailureReport() tea.Cmd {
	if m.prepare.reportRequested || m.prepare.loading() || !m.prepare.phpDone {
		return nil
	}
	if m.prepare.resolve == nil || m.prepare.resolve.OK {
		return nil
	}
	m.prepare.reportRequested = true
	return exportReportCmd(m.upgrader, m.reportData())
}

func (m *Model) updatePrepareKeys(msg tea.KeyPressMsg) (app.Content, tea.Cmd) {
	key := app.KeyString(msg)

	switch key {
	case "up", "k", "down", "j":
		// The cursor moves over the extension rows and, one step past the
		// last row, onto the Continue button.
		m.prepare.cursor = tui.MoveCursor(m.prepare.cursor, key, len(m.prepare.results)+1)
		m.clampPrepareScroll()
	case "right", "l":
		// The Continue button lives in the right column.
		if m.prepare.cursor < len(m.prepare.results) {
			m.prepare.queueRow = m.prepare.cursor
			m.prepare.cursor = len(m.prepare.results)
		}
	case "left", "h":
		if m.prepare.cursor >= len(m.prepare.results) && len(m.prepare.results) > 0 {
			m.prepare.cursor = min(m.prepare.queueRow, len(m.prepare.results)-1)
			m.clampPrepareScroll()
		}
	case "enter":
		if m.prepare.cursor < len(m.prepare.results) {
			detail := newExtensionDetail(m.prepare.results[m.prepare.cursor], m.targetLabel())
			return m, m.host.PushOverlay(&detail)
		}
		// Enter on the focused Continue button.
		if m.prepare.ready() {
			return m.beginReview()
		}
	case "r":
		return m.beginPrepare()
	case "c":
		if m.prepare.ready() {
			return m.beginReview()
		}
	case "esc":
		m.panel = panelCheck
		return m, nil
	case "q":
		return m, tea.Quit
	}
	return m, nil
}

// queueHeight is the number of extension rows visible in the queue.
func (m *Model) queueHeight() int {
	h := m.bodyHeight(true) - 10 // headings, system checks, table header
	if h < 3 {
		return 3
	}
	return h
}

func (m *Model) clampPrepareScroll() {
	visible := m.queueHeight()
	// The cursor position past the last row focuses the Continue button and
	// does not scroll the queue.
	row := min(m.prepare.cursor, len(m.prepare.results)-1)
	if row < 0 {
		row = 0
	}
	if row < m.prepare.scroll {
		m.prepare.scroll = row
	}
	if row >= m.prepare.scroll+visible {
		m.prepare.scroll = row - visible + 1
	}
	if m.prepare.scroll < 0 {
		m.prepare.scroll = 0
	}
}

func (m *Model) viewPrepare() (title, status, body string) {
	title = "Prepare upgrade"

	switch {
	case m.prepare.loading():
		status = m.statusStrip(tui.VariantInfo, "RUNNING", "Running preparation checks…")
	case !m.prepare.ready() && m.prepare.flagged() > 0:
		status = m.statusStrip(tui.VariantError, "BLOCKED",
			fmt.Sprintf("Composer cannot resolve this upgrade — %d extensions need attention.", m.prepare.flagged())) +
			"\n" + m.statusStrip(tui.VariantInfo, "TODO", "Fix manually if needed, then recheck."+m.reportHint())
	case !m.prepare.ready():
		status = m.statusStrip(tui.VariantError, "BLOCKED", "Composer cannot resolve this upgrade. The conflict summary is below."+m.reportHint())
	case m.prepare.blockers() > 0:
		status = m.statusStrip(tui.VariantWarning, "REVIEW",
			fmt.Sprintf("Composer resolved the upgrade, but %d extensions are flagged. Review them before continuing.", m.prepare.blockers()))
	default:
		status = m.statusStrip(tui.VariantSuccess, "READY", "All checks passed. Continue to review the upgrade plan.")
	}

	body = m.twoColumn(m.bodyWidth()*3/5, m.viewPrepareLeft(), m.viewPrepareRight())
	return title, status, body
}

// reportHint appends the failure report's location — or its write failure —
// to a status message.
func (m *Model) reportHint() string {
	switch {
	case m.prepare.reportPath != "":
		return " Report: " + relativePath(m.opts.ProjectRoot, m.prepare.reportPath)
	case m.prepare.reportErr != nil:
		return " Report write failed: " + m.prepare.reportErr.Error()
	}
	return ""
}

func (m *Model) viewPrepareLeft() string {
	var b strings.Builder

	b.WriteString(tui.BoldStyle.Render("Upgrade checks"))
	b.WriteString("\n")
	b.WriteString(m.renderSystemChecks())
	b.WriteString("\n")

	// A pure Composer conflict shows the solver's conflict summary where the
	// queue would be — it names the exact packages and constraints that
	// clash. Any flagged extension (blocking, deprecated, manual review)
	// keeps the queue instead: those findings are only visible here.
	if m.prepare.resolve != nil && !m.prepare.resolve.OK && m.prepare.flagged() == 0 {
		b.WriteString(m.viewResolveFailure())
		return b.String()
	}

	b.WriteString(tui.BoldStyle.Render("Extension queue"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("Blocking extensions are listed first."))
	b.WriteString("\n")

	if !m.prepare.compatDone {
		b.WriteString(tui.DimStyle.Render("Checking extension compatibility…"))
		return b.String()
	}
	if len(m.prepare.results) == 0 {
		b.WriteString(tui.DimStyle.Render("No extensions found."))
		return b.String()
	}

	nameW, versionW := 26, 20
	b.WriteString("    " + tui.BoldStyle.Render(tui.PadRight("Name", nameW)+tui.PadRight("Current -> target", versionW)+"Result"))
	b.WriteString("\n")

	visible := m.queueHeight()
	end := min(m.prepare.scroll+visible, len(m.prepare.results))

	rows := make([]string, 0, visible)
	for i := m.prepare.scroll; i < end; i++ {
		rows = append(rows, extensionQueueRow(m.prepare.results[i], i == m.prepare.cursor, nameW, versionW))
	}

	table := strings.Join(rows, "\n")
	bar := tui.NewScrollbar(tui.ScrollbarOptions{
		Total: len(m.prepare.results), Visible: visible, Offset: m.prepare.scroll, Height: len(rows),
	}).Render()
	if bar != "" {
		table = tui.JoinColumns(table, bar, 1)
	}
	b.WriteString(table)

	return b.String()
}

// viewResolveFailure renders the tail of Composer's output — the solver ends
// with its "Problem 1: …" conflict summary, which is the actionable part.
func (m *Model) viewResolveFailure() string {
	var b strings.Builder
	b.WriteString(tui.BoldStyle.Render("Composer conflict"))
	b.WriteString("\n")

	width := m.bodyWidth() * 3 / 5
	// The section must fit where the queue block would render (heading, hint,
	// table header, plus queueHeight rows): heading + omission notice + tail
	// + blank + report line. Budget the tail so the report link is never
	// cropped off the frame — long failures need it the most.
	visible := m.queueHeight() - 2
	if visible < 3 {
		visible = 3
	}

	lines := strings.Split(strings.TrimRight(m.prepare.resolve.Report, "\n"), "\n")
	if len(lines) > visible {
		b.WriteString(tui.DimStyle.Render(fmt.Sprintf("… %d earlier output lines omitted", len(lines)-visible)))
		b.WriteString("\n")
	}
	for _, line := range tui.TailLines(lines, visible) {
		b.WriteString(tui.Truncate(line, width))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	switch {
	case m.prepare.reportPath != "":
		b.WriteString(tui.DimStyle.Render("Full output: ") +
			tui.StyledLink("file://"+m.prepare.reportPath, relativePath(m.opts.ProjectRoot, m.prepare.reportPath), tui.LinkStyle))
	case m.prepare.reportErr != nil:
		b.WriteString(failStyle.Render(tui.Truncate("Could not write the report: "+m.prepare.reportErr.Error(), width)))
	}
	return b.String()
}

func (m *Model) renderSystemChecks() string {
	var b strings.Builder

	row := func(label string, state upgrade.CheckState, value string) {
		var style lipgloss.Style
		switch state {
		case upgrade.StateFail:
			style = failStyle
		case upgrade.StateWarn:
			style = warnStyle
		case upgrade.StatePending, upgrade.StateRunning:
			style = tui.DimStyle
		case upgrade.StateOK:
			style = okStyle
		default:
			style = okStyle
		}
		b.WriteString(tui.NewCheckRow(tui.CheckRowOptions{
			State: dotState(state), Label: label, Value: style.Render(value), LabelWidth: 36,
		}).Render())
		b.WriteString("\n")
	}

	envLabel := "Web service running"
	if m.opts.EnvName != "" {
		envLabel = strings.ToUpper(m.opts.EnvName[:1]) + m.opts.EnvName[1:] + " web service running"
	}
	switch {
	case m.prepare.envRunning == nil:
		row(envLabel, upgrade.StatePending, "checking…")
	case m.prepare.envErr != nil:
		row(envLabel, upgrade.StateWarn, "unknown")
	case *m.prepare.envRunning:
		row(envLabel, upgrade.StateOK, "yes")
	default:
		row(envLabel, upgrade.StateWarn, "no — start it before the upgrade runs")
	}

	switch {
	case m.prepare.packagist == nil:
		row("Packagist reachable", upgrade.StatePending, "checking…")
	case *m.prepare.packagist:
		row("Packagist reachable", upgrade.StateOK, "yes")
	default:
		row("Packagist reachable", upgrade.StateFail, "no")
	}

	switch {
	case m.prepare.resolveErr != nil:
		row("Composer can resolve this upgrade", upgrade.StateFail, "error")
	case m.prepare.resolve == nil:
		row("Composer can resolve this upgrade", upgrade.StatePending, "checking…")
	case m.prepare.resolve.OK:
		row("Composer can resolve this upgrade", upgrade.StateOK, "yes")
	default:
		row("Composer can resolve this upgrade", upgrade.StateFail, "blocked")
	}

	switch {
	case !m.prepare.compatDone:
		row("Extension compatibility", upgrade.StatePending, "checking…")
	case m.prepare.blockers() > 0 && m.prepare.resolve != nil && m.prepare.resolve.OK:
		row("Extension compatibility", upgrade.StateWarn, fmt.Sprintf("%d flagged", m.prepare.blockers()))
	case m.prepare.blockers() > 0:
		row("Extension compatibility", upgrade.StateFail, fmt.Sprintf("%d blockers", m.prepare.blockers()))
	default:
		row("Extension compatibility", upgrade.StateOK, "ok")
	}

	return b.String()
}

func (m *Model) viewPrepareRight() string {
	var b strings.Builder
	b.WriteString(tui.BoldStyle.Render("Deployment Helper workflow"))
	b.WriteString("\n\n")
	b.WriteString(tui.DimStyle.Render("  • ") + tui.LabelStyle.Render("add shopware/deployment-helper"))
	b.WriteString("\n")
	b.WriteString(tui.LabelStyle.Render("    if missing"))
	b.WriteString("\n\n\n")
	b.WriteString(userActionStyle.Render("User action"))
	b.WriteString("\n")
	b.WriteString(tui.LabelStyle.Render("Open an extension detail popup to"))
	b.WriteString("\n")
	b.WriteString(tui.LabelStyle.Render("review, fix, or confirm an item."))
	b.WriteString("\n\n")
	b.WriteString(tui.DimStyle.Render("No project files change in this step."))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("Changes happen only after Review."))
	b.WriteString("\n\n")

	// Blue marks the focused button (as everywhere else); whether it does
	// anything is communicated by the READY/BLOCKED status strip.
	focused := m.prepare.cursor >= len(m.prepare.results)
	active := -1
	cursor := "  "
	if focused {
		cursor = userActionStyle.Render("> ")
		if m.prepare.ready() {
			active = 0
		}
	}
	b.WriteString(cursor + m.buttonRow([]string{"Continue"}, active))
	return b.String()
}

// versionTransition renders the "Current -> target" queue column.
func versionTransition(r upgrade.ExtensionResult) string {
	if !r.Extension.ComposerManaged {
		return "local"
	}
	to := r.Available
	if to == "" {
		to = "none"
	}
	return r.Extension.Version + " -> " + to
}
