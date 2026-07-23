package upgradetui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/shopware/shopware-cli/internal/shop/upgrade"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/tui/app"
	"github.com/shopware/shopware-cli/internal/tui/picker"
)

// checkState backs panel 2: the readiness checklist (left) and target version
// selection (right).
type checkState struct {
	loading    bool
	readiness  upgrade.Readiness
	catalog    *upgrade.Catalog
	catalogErr error

	cursor int // index into versionRows()
	chosen *upgrade.VersionOption
}

func newCheckState() checkState {
	return checkState{}
}

// target returns the version the wizard will upgrade to, nil while undecided.
func (s checkState) target() *upgrade.VersionOption {
	return s.chosen
}

// versionRow is one selectable entry in the right column.
type versionRow struct {
	option *upgrade.VersionOption // nil for the "choose another" row
	label  string
	hint   string
}

// versionRows lists the quick choices: recommended, latest patch of the
// current minor, and the full picker.
func (s checkState) versionRows() []versionRow {
	var rows []versionRow
	if s.catalog != nil {
		if s.catalog.Recommended >= 0 {
			opt := &s.catalog.Options[s.catalog.Recommended]
			rows = append(rows, versionRow{option: opt, label: opt.Version.String(), hint: "recommended"})
		}
		if s.catalog.LatestPatch >= 0 && s.catalog.LatestPatch != s.catalog.Recommended {
			opt := &s.catalog.Options[s.catalog.LatestPatch]
			rows = append(rows, versionRow{option: opt, label: opt.Version.String(), hint: opt.Tag})
		}
	}
	rows = append(rows, versionRow{label: "Choose another supported version…"})
	return rows
}

func (m *Model) updateCheck(msg tea.Msg) (app.Content, tea.Cmd) {
	switch msg := msg.(type) {
	case checksDoneMsg:
		m.check.loading = false
		m.check.readiness = msg.readiness
		return m, loadCatalogCmd(m.upgrader, msg.readiness)

	case catalogLoadedMsg:
		m.check.catalog = msg.catalog
		m.check.catalogErr = msg.err
		if msg.err == nil && msg.catalog != nil && msg.catalog.Recommended >= 0 {
			m.check.chosen = &msg.catalog.Options[msg.catalog.Recommended]
		}
		return m, nil

	case picker.ResultMsg:
		if _, ok := msg.Key.(versionPickerKey); ok && !msg.Cancelled {
			option := m.check.catalog.Options[msg.Index]
			m.check.chosen = &option
			// The picker's confirm is the selection: continue directly instead
			// of landing back on the "Choose another" row, where Enter would
			// reopen the picker. When checks still block, this stays on the
			// panel with the custom choice displayed.
			return m.continueToPrepare()
		}
		return m, nil

	case tea.KeyPressMsg:
		return m.updateCheckKeys(msg)
	}
	return m, nil
}

func (m *Model) updateCheckKeys(msg tea.KeyPressMsg) (app.Content, tea.Cmd) {
	key := app.KeyString(msg)
	rows := m.check.versionRows()

	switch key {
	case "up", "k", "down", "j":
		// The cursor is focus only; the selected version (◉) changes when a
		// row is activated with Enter, never while navigating.
		m.check.cursor = tui.MoveCursor(m.check.cursor, key, len(rows))
	case "r":
		return m.recheck()
	case "q", "esc":
		return m, tea.Quit
	case "enter":
		row := rows[m.check.cursor]
		if row.option == nil {
			return m.openVersionPicker()
		}
		m.check.chosen = row.option
		return m.continueToPrepare()
	}
	return m, nil
}

func (m *Model) recheck() (app.Content, tea.Cmd) {
	m.check.loading = true
	return m, runChecksCmd(m.upgrader)
}

func (m *Model) openVersionPicker() (app.Content, tea.Cmd) {
	if m.check.catalog == nil || len(m.check.catalog.Options) == 0 {
		return m, nil
	}
	return m, m.host.PushOverlay(newVersionPicker(m.check.catalog))
}

// continueToPrepare enters panel 3 once nothing blocks it.
func (m *Model) continueToPrepare() (app.Content, tea.Cmd) {
	if m.check.loading || m.check.readiness.Blocked() || m.check.target() == nil {
		return m, nil
	}
	return m.beginPrepare()
}

func (m *Model) viewCheck() (title, status, body string) {
	title = "Check project + choose Shopware version"

	switch {
	case m.check.loading:
		status = m.statusStrip(tui.VariantInfo, "RUNNING", "Checking the project…")
	case m.check.readiness.Blocked():
		status = m.statusStrip(tui.VariantError, "BLOCKED", "Fix the failing checks below, then press Recheck.")
	}

	body = m.twoColumn(m.bodyWidth()*11/20, m.viewCheckLeft(), m.viewCheckRight())
	return title, status, body
}

func (m *Model) viewCheckLeft() string {
	var b strings.Builder
	b.WriteString(tui.BoldStyle.Render("1. Check project readiness"))
	b.WriteString("\n\n")
	b.WriteString(tui.DimStyle.Render("These checks are read-only."))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("If something fails, fix it and press Recheck."))
	b.WriteString("\n\n")

	if m.check.loading && len(m.check.readiness.Checks) == 0 {
		b.WriteString(tui.DimStyle.Render("Running checks…"))
		return b.String()
	}

	labelWidth := 38
	for _, check := range m.check.readiness.Checks {
		b.WriteString(tui.NewCheckRow(tui.CheckRowOptions{
			State: dotState(check.State), Label: check.Label, Value: tui.BoldStyle.Render(check.Value), LabelWidth: labelWidth,
		}).Render())
		b.WriteString("\n")
		if check.Detail != "" && check.State != upgrade.StateOK {
			b.WriteString(tui.DimStyle.Render("   " + check.Detail))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(tui.BoldStyle.Render("Status"))
	b.WriteString("\n")
	switch {
	case m.check.loading:
		b.WriteString(tui.DimStyle.Render("Checking…"))
	case m.check.readiness.Blocked():
		b.WriteString(failStyle.Bold(true).Render("Fix the blocking checks before choosing a version."))
	default:
		b.WriteString(okStyle.Bold(true).Render("Project is ready. Choose a Shopware version next."))
	}

	return b.String()
}

func (m *Model) viewCheckRight() string {
	var b strings.Builder
	b.WriteString(userActionStyle.Render("User action"))
	b.WriteString("\n\n")
	b.WriteString(tui.BoldStyle.Render("2. Choose Shopware version"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("Select the version this project should"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("upgrade to."))
	b.WriteString("\n\n")

	switch {
	case m.check.catalogErr != nil:
		b.WriteString(failStyle.Render("Could not load versions: " + m.check.catalogErr.Error()))
		b.WriteString("\n")
	case m.check.catalog == nil:
		b.WriteString(tui.DimStyle.Render("Loading available versions…"))
		b.WriteString("\n")
	case len(m.check.catalog.Options) == 0:
		b.WriteString(okStyle.Render("You are already on the latest version of Shopware."))
		b.WriteString("\n")
	default:
		for i, row := range m.check.versionRows() {
			// Exactly one row carries the ◉: the row whose version is the
			// current target — the custom row when a picked version is not
			// one of the quick choices.
			marker := "○"
			switch {
			case row.option != nil && m.check.chosen != nil && row.option.Version.Equal(m.check.chosen.Version):
				marker = okStyle.Render("◉")
			case row.option == nil && m.check.chosen != nil && !m.isQuickChoice(m.check.chosen):
				marker = okStyle.Render("◉")
			}
			cursor := "  "
			if i == m.check.cursor {
				cursor = userActionStyle.Render("> ")
			}

			if row.option == nil {
				b.WriteString(cursor + marker + " " + tui.LabelStyle.Render(row.label))
				if m.check.chosen != nil && !m.isQuickChoice(m.check.chosen) {
					b.WriteString(" " + tui.BoldStyle.Render(m.check.chosen.Version.String()))
				}
				b.WriteString("\n")
				continue
			}

			link := tui.StyledLink(row.option.ReleaseNotesURL, row.label, tui.LinkStyle)
			b.WriteString(cursor + marker + " " + link + "   " + tui.LabelStyle.Render(row.hint))
			b.WriteString("\n")
			if detail := supportDetail(*row.option); detail != "" {
				b.WriteString(tui.DimStyle.Render("       " + detail))
				b.WriteString("\n")
			}
		}

		if t := m.check.target(); t != nil && m.check.readiness.CurrentVersion != nil &&
			upgrade.IsMultiMajorJump(m.check.readiness.CurrentVersion, t.Version) {
			b.WriteString("\n")
			b.WriteString(warnStyle.Render("This path spans multiple release lines —"))
			b.WriteString("\n")
			b.WriteString(warnStyle.Render("vendors usually validate one at a time."))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(tui.BoldStyle.Render("Controls"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("↑/↓ moves the selection."))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("Enter applies it."))
	b.WriteString("\n\n")
	b.WriteString(tui.BoldStyle.Render("Next"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("Enter checks extension compatibility"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("for the selected version."))

	return b.String()
}

// isQuickChoice reports whether the chosen version is one of the two quick
// rows (so the "choose another" row does not repeat it).
func (m *Model) isQuickChoice(chosen *upgrade.VersionOption) bool {
	for _, row := range m.check.versionRows() {
		if row.option != nil && row.option.Version.Equal(chosen.Version) {
			return true
		}
	}
	return false
}

func supportDetail(o upgrade.VersionOption) string {
	left := o.SupportLeft()
	if left == "" {
		return ""
	}
	kind := o.SupportType
	if kind == "" || kind == "active" {
		kind = "support"
	}
	return fmt.Sprintf("%s: %s left", kind, left)
}
