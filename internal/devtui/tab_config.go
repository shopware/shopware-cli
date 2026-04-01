package devtui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/tui"
)

// configField identifies an editable field in the config tab.
type configField int

const (
	fieldPHPVersion configField = iota
	fieldProfiler
	fieldBlackfireServerID
	fieldBlackfireServerToken
	fieldTidewaysAPIKey
	fieldNodeVersion
	fieldSave
	fieldCount // sentinel – number of fields
)

var (
	phpVersions  = []string{"8.2", "8.3", "8.4"}
	nodeVersions = []string{"22", "24"}
	profilers    = []string{"", "xdebug", "blackfire", "tideways", "pcov", "spx"}
)

// ConfigModel holds the state for the Environment Config tab.
type ConfigModel struct {
	cursor configField

	phpVersion  int // index into phpVersions
	nodeVersion int // index into nodeVersions
	profiler    int // index into profilers

	blackfireServerID    textinput.Model
	blackfireServerToken textinput.Model
	tidewaysAPIKey       textinput.Model

	editing  bool // true when a text input is focused
	saved    bool // flash a "saved" indicator
	modified bool // config has unsaved changes

	width  int
	height int
}

type configSavedMsg struct{}

func NewConfigModel(cfg *shop.Config) ConfigModel {
	m := ConfigModel{}

	if cfg != nil && cfg.Docker != nil {
		if cfg.Docker.PHP != nil {
			m.phpVersion = indexOf(phpVersions, cfg.Docker.PHP.Version, 1) // default 8.3
			m.profiler = indexOf(profilers, cfg.Docker.PHP.Profiler, 0)
			m.blackfireServerID = newConfigInput("Server ID", cfg.Docker.PHP.BlackfireServerID)
			m.blackfireServerToken = newConfigInput("Server Token", cfg.Docker.PHP.BlackfireServerToken)
			m.tidewaysAPIKey = newConfigInput("API Key", cfg.Docker.PHP.TidewaysAPIKey)
		}
		if cfg.Docker.Node != nil {
			m.nodeVersion = indexOf(nodeVersions, cfg.Docker.Node.Version, 0)
		}
	}

	// Ensure text inputs are initialized even if config was nil.
	if m.blackfireServerID.Placeholder == "" {
		m.blackfireServerID = newConfigInput("Server ID", "")
	}
	if m.blackfireServerToken.Placeholder == "" {
		m.blackfireServerToken = newConfigInput("Server Token", "")
	}
	if m.tidewaysAPIKey.Placeholder == "" {
		m.tidewaysAPIKey = newConfigInput("API Key", "")
	}

	return m
}

func newConfigInput(placeholder, value string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 128
	ti.Prompt = ""
	if value != "" {
		ti.SetValue(value)
	}
	return ti
}

func indexOf(slice []string, value string, defaultIdx int) int {
	for i, v := range slice {
		if v == value {
			return i
		}
	}
	return defaultIdx
}

func (m *ConfigModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

func (m ConfigModel) Update(msg tea.Msg) (ConfigModel, tea.Cmd) {
	if _, ok := msg.(configSavedMsg); ok {
		m.saved = true
		m.modified = false
		return m, nil
	}
	return m, nil
}

func (m ConfigModel) HandleKey(msg tea.KeyPressMsg) (ConfigModel, tea.Cmd) {
	key := msg.String()

	// When editing a text field, route keys to the active input.
	if m.editing {
		switch key {
		case keyEnter, "esc":
			m.blurAll()
			m.editing = false
			return m, nil
		default:
			var cmd tea.Cmd
			switch m.cursor {
			case fieldBlackfireServerID:
				m.blackfireServerID, cmd = m.blackfireServerID.Update(msg)
			case fieldBlackfireServerToken:
				m.blackfireServerToken, cmd = m.blackfireServerToken.Update(msg)
			case fieldTidewaysAPIKey:
				m.tidewaysAPIKey, cmd = m.tidewaysAPIKey.Update(msg)
			}
			m.modified = true
			return m, cmd
		}
	}

	switch key {
	case keyUp, keyK:
		m.moveCursorUp()
	case keyDown, keyJ:
		m.moveCursorDown()
	case keyEnter, " ":
		return m.activateField()
	case "left", "h":
		m.cyclePrev()
	case "right", "l":
		m.cycleNext()
	}

	return m, nil
}

func (m *ConfigModel) moveCursorUp() {
	for {
		if m.cursor <= 0 {
			return
		}
		m.cursor--
		if m.isFieldVisible(m.cursor) {
			return
		}
	}
}

func (m *ConfigModel) moveCursorDown() {
	for {
		if m.cursor >= fieldCount-1 {
			return
		}
		m.cursor++
		if m.isFieldVisible(m.cursor) {
			return
		}
	}
}

func (m ConfigModel) isFieldVisible(f configField) bool {
	profilerName := profilers[m.profiler]
	switch f {
	case fieldBlackfireServerID, fieldBlackfireServerToken:
		return profilerName == "blackfire"
	case fieldTidewaysAPIKey:
		return profilerName == "tideways"
	default:
		return true
	}
}

func (m *ConfigModel) activateField() (ConfigModel, tea.Cmd) {
	switch m.cursor {
	case fieldPHPVersion:
		m.phpVersion = (m.phpVersion + 1) % len(phpVersions)
		m.modified = true
	case fieldNodeVersion:
		m.nodeVersion = (m.nodeVersion + 1) % len(nodeVersions)
		m.modified = true
	case fieldProfiler:
		m.profiler = (m.profiler + 1) % len(profilers)
		m.modified = true
		// Re-validate cursor position after profiler change.
		if !m.isFieldVisible(m.cursor) {
			m.moveCursorDown()
		}
	case fieldBlackfireServerID:
		m.editing = true
		m.blackfireServerID.Focus()
		return *m, textinput.Blink
	case fieldBlackfireServerToken:
		m.editing = true
		m.blackfireServerToken.Focus()
		return *m, textinput.Blink
	case fieldTidewaysAPIKey:
		m.editing = true
		m.tidewaysAPIKey.Focus()
		return *m, textinput.Blink
	case fieldSave:
		// Save is handled by the parent model.
		return *m, nil
	}
	return *m, nil
}

func (m *ConfigModel) cyclePrev() {
	m.saved = false
	switch m.cursor {
	case fieldPHPVersion:
		m.phpVersion = (m.phpVersion - 1 + len(phpVersions)) % len(phpVersions)
		m.modified = true
	case fieldNodeVersion:
		m.nodeVersion = (m.nodeVersion - 1 + len(nodeVersions)) % len(nodeVersions)
		m.modified = true
	case fieldProfiler:
		m.profiler = (m.profiler - 1 + len(profilers)) % len(profilers)
		m.modified = true
	}
}

func (m *ConfigModel) cycleNext() {
	m.saved = false
	switch m.cursor {
	case fieldPHPVersion:
		m.phpVersion = (m.phpVersion + 1) % len(phpVersions)
		m.modified = true
	case fieldNodeVersion:
		m.nodeVersion = (m.nodeVersion + 1) % len(nodeVersions)
		m.modified = true
	case fieldProfiler:
		m.profiler = (m.profiler + 1) % len(profilers)
		m.modified = true
	}
}

func (m *ConfigModel) blurAll() {
	m.blackfireServerID.Blur()
	m.blackfireServerToken.Blur()
	m.tidewaysAPIKey.Blur()
}

// ApplyToConfig writes the non-sensitive form values into the given Config.
// Credentials are excluded — use LocalConfig to obtain them separately.
func (m ConfigModel) ApplyToConfig(cfg *shop.Config) {
	if cfg.Docker == nil {
		cfg.Docker = &shop.ConfigDocker{}
	}
	if cfg.Docker.PHP == nil {
		cfg.Docker.PHP = &shop.ConfigDockerPHP{}
	}
	if cfg.Docker.Node == nil {
		cfg.Docker.Node = &shop.ConfigDockerNode{}
	}

	cfg.Docker.PHP.Version = phpVersions[m.phpVersion]
	cfg.Docker.Node.Version = nodeVersions[m.nodeVersion]
	cfg.Docker.PHP.Profiler = profilers[m.profiler]

	// Credentials are stored in the local config file, not here.
	cfg.Docker.PHP.BlackfireServerID = ""
	cfg.Docker.PHP.BlackfireServerToken = ""
	cfg.Docker.PHP.TidewaysAPIKey = ""
}

// LocalConfig returns a partial Config containing only the sensitive
// credential fields for the active profiler. This is written to
// .shopware-project.local.yml so secrets stay out of version control.
func (m ConfigModel) LocalConfig() *shop.Config {
	php := &shop.ConfigDockerPHP{}

	switch profilers[m.profiler] {
	case "blackfire":
		php.BlackfireServerID = m.blackfireServerID.Value()
		php.BlackfireServerToken = m.blackfireServerToken.Value()
	case "tideways":
		php.TidewaysAPIKey = m.tidewaysAPIKey.Value()
	default:
		return nil // no credentials to persist
	}

	return &shop.Config{
		Docker: &shop.ConfigDocker{
			PHP: php,
		},
	}
}

func (m ConfigModel) View(width, height int) string {
	var s strings.Builder

	selectedArrow := lipgloss.NewStyle().Foreground(tui.BrandColor).Render("▸ ")
	normalIndent := "  "

	divider := tui.SectionDivider(width - 8)

	// --- PHP Settings ---
	s.WriteString(tui.TitleStyle.Render("PHP"))
	s.WriteString("\n")
	s.WriteString(m.renderSelect(fieldPHPVersion, "Version", phpVersions[m.phpVersion], selectedArrow, normalIndent))
	s.WriteString(m.renderSelect(fieldProfiler, "Profiler", m.profilerLabel(), selectedArrow, normalIndent))

	// Profiler-specific credential fields.
	profilerName := profilers[m.profiler]
	if profilerName == "blackfire" {
		s.WriteString(m.renderInput(fieldBlackfireServerID, "Server ID", m.blackfireServerID, selectedArrow, normalIndent))
		s.WriteString(m.renderInput(fieldBlackfireServerToken, "Server Token", m.blackfireServerToken, selectedArrow, normalIndent))
	}
	if profilerName == "tideways" {
		s.WriteString(m.renderInput(fieldTidewaysAPIKey, "API Key", m.tidewaysAPIKey, selectedArrow, normalIndent))
	}

	s.WriteString(divider)

	// --- Node Settings ---
	s.WriteString(tui.TitleStyle.Render("Node.js"))
	s.WriteString("\n")
	s.WriteString(m.renderSelect(fieldNodeVersion, "Version", nodeVersions[m.nodeVersion], selectedArrow, normalIndent))

	s.WriteString(divider)

	// --- Save Button ---
	if m.cursor == fieldSave {
		s.WriteString(selectedArrow)
	} else {
		s.WriteString(normalIndent)
	}
	if m.saved {
		s.WriteString(activeBadgeStyle.Render("Saved"))
		s.WriteString("  ")
		s.WriteString(helpStyle.Render("Restart Docker to apply changes."))
	} else if m.modified {
		if m.cursor == fieldSave {
			s.WriteString(activeBtnStyle.Render("Save & Regenerate"))
		} else {
			s.WriteString(warningBadgeStyle.Render("unsaved changes"))
			s.WriteString("  ")
			s.WriteString(helpStyle.Render("Navigate to Save & Regenerate"))
		}
	} else {
		s.WriteString(helpStyle.Render("No changes"))
	}
	s.WriteString("\n")

	return s.String()
}

func (m ConfigModel) profilerLabel() string {
	p := profilers[m.profiler]
	if p == "" {
		return "none"
	}
	return p
}

func (m ConfigModel) renderSelect(field configField, label, value, selectedArrow, normalIndent string) string {
	prefix := normalIndent
	if m.cursor == field {
		prefix = selectedArrow
	}

	valStyle := valueStyle
	if m.cursor == field {
		valStyle = lipgloss.NewStyle().Foreground(tui.BrandColor).Bold(true)
	}

	arrows := ""
	if m.cursor == field {
		arrows = helpStyle.Render(" ◂ ▸")
	}

	return fmt.Sprintf("%s%-20s%s%s\n", prefix, tui.LabelStyle.Render(label), valStyle.Render(value), arrows)
}

func (m ConfigModel) renderInput(field configField, label string, input textinput.Model, selectedArrow, normalIndent string) string {
	prefix := normalIndent
	if m.cursor == field {
		prefix = selectedArrow
	}

	if m.cursor == field && m.editing {
		return fmt.Sprintf("%s%-20s%s\n", prefix, tui.LabelStyle.Render(label), input.View())
	}

	val := input.Value()
	if val == "" {
		val = helpStyle.Render("(not set)")
	} else {
		val = secretStyle.Render(val)
	}

	hint := ""
	if m.cursor == field {
		hint = helpStyle.Render(" enter to edit")
	}

	return fmt.Sprintf("%s%-20s%s%s\n", prefix, tui.LabelStyle.Render(label), val, hint)
}
