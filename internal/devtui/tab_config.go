package devtui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	dockerpkg "github.com/shopware/shopware-cli/internal/docker"
	"github.com/shopware/shopware-cli/internal/packagist"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/tui"
)

type configField int

const (
	fieldAppEnv configField = iota
	fieldPHPVersion
	fieldProfiler
	fieldBlackfireServerID
	fieldBlackfireServerToken
	fieldTidewaysAPIKey
	fieldSave
	fieldCount
)

const (
	profilerBlackfire = dockerpkg.ProfilerBlackfire
	profilerTideways  = dockerpkg.ProfilerTideways

	defaultPHPVersionIndex = 1
	defaultAppEnvIndex     = 0
)

var (
	phpVersions = packagist.SupportedPHPVersions
	profilers   = dockerpkg.Profilers
	appEnvs     = []string{"dev", "prod", "test"}
)

type ConfigModel struct {
	cursor configField

	appEnv         int
	originalAppEnv string
	phpVersion     int
	profiler       int

	blackfireServerID    textinput.Model
	blackfireServerToken textinput.Model
	tidewaysAPIKey       textinput.Model

	saved    bool
	modified bool
	err      error

	width  int
	height int
}

func NewConfigModel(cfg *shop.Config, appEnv string) ConfigModel {
	resolvedAppEnv := appEnv
	if resolvedAppEnv == "" {
		resolvedAppEnv = appEnvs[defaultAppEnvIndex]
	}
	m := ConfigModel{
		cursor:         fieldAppEnv,
		appEnv:         indexOf(appEnvs, resolvedAppEnv, defaultAppEnvIndex),
		originalAppEnv: resolvedAppEnv,
		phpVersion:     defaultPHPVersionIndex,
	}

	if cfg != nil && cfg.Docker != nil && cfg.Docker.PHP != nil {
		m.phpVersion = indexOf(phpVersions, cfg.Docker.PHP.Version, defaultPHPVersionIndex)
		m.profiler = indexOf(profilers, cfg.Docker.PHP.Profiler, 0)
		m.blackfireServerID = newConfigInput("Server ID", cfg.Docker.PHP.BlackfireServerID)
		m.blackfireServerToken = newConfigInput("Server Token", cfg.Docker.PHP.BlackfireServerToken)
		m.tidewaysAPIKey = newConfigInput("API Key", cfg.Docker.PHP.TidewaysAPIKey)
	}

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
	return m, nil
}

func (m ConfigModel) HandleKey(msg tea.KeyPressMsg) (ConfigModel, tea.Cmd) {
	switch msg.String() {
	case keyUp, keyK:
		m.moveCursorUp()
	case keyDown, keyJ:
		m.moveCursorDown()
	}

	return m, nil
}

func (m ConfigModel) PickerForCursor() Modal {
	switch m.cursor { //nolint:exhaustive
	case fieldAppEnv:
		items := make([]listPickerItem, len(appEnvs))
		for i, v := range appEnvs {
			items[i] = listPickerItem{Label: v, Value: v}
		}
		return newListPicker(fieldAppEnv, "APP_ENV", "Symfony application environment (.env.local)", items, m.appEnv)
	case fieldPHPVersion:
		items := make([]listPickerItem, len(phpVersions))
		for i, v := range phpVersions {
			items[i] = listPickerItem{Label: v, Value: v}
		}
		return newListPicker(fieldPHPVersion, "PHP Version", "", items, m.phpVersion)
	case fieldProfiler:
		items := make([]listPickerItem, len(profilers))
		for i, p := range profilers {
			label := p
			if p == "" {
				label = "none"
			}
			items[i] = listPickerItem{Label: label, Value: p}
		}
		return newListPicker(fieldProfiler, "PHP Profiler", "", items, m.profiler)
	case fieldBlackfireServerID:
		return newTextPicker(fieldBlackfireServerID, "Blackfire Server ID", "Server ID for the Blackfire profiler", m.blackfireServerID.Value(), true)
	case fieldBlackfireServerToken:
		return newTextPicker(fieldBlackfireServerToken, "Blackfire Server Token", "Server token for the Blackfire profiler", m.blackfireServerToken.Value(), true)
	case fieldTidewaysAPIKey:
		return newTextPicker(fieldTidewaysAPIKey, "Tideways API Key", "API key for Tideways", m.tidewaysAPIKey.Value(), true)
	}
	return nil
}

func (m *ConfigModel) ApplyPickerValue(field configField, value string) bool {
	changed := false
	switch field { //nolint:exhaustive
	case fieldAppEnv:
		idx := indexOf(appEnvs, value, m.appEnv)
		if idx != m.appEnv {
			m.appEnv = idx
			changed = true
		}
	case fieldPHPVersion:
		idx := indexOf(phpVersions, value, m.phpVersion)
		if idx != m.phpVersion {
			m.phpVersion = idx
			changed = true
		}
	case fieldProfiler:
		idx := indexOf(profilers, value, m.profiler)
		if idx != m.profiler {
			m.profiler = idx
			changed = true
			if !m.isFieldVisible(m.cursor) {
				m.moveCursorDown()
			}
		}
	case fieldBlackfireServerID:
		if m.blackfireServerID.Value() != value {
			m.blackfireServerID.SetValue(value)
			changed = true
		}
	case fieldBlackfireServerToken:
		if m.blackfireServerToken.Value() != value {
			m.blackfireServerToken.SetValue(value)
			changed = true
		}
	case fieldTidewaysAPIKey:
		if m.tidewaysAPIKey.Value() != value {
			m.tidewaysAPIKey.SetValue(value)
			changed = true
		}
	}
	if changed {
		m.modified = true
		m.saved = false
		m.err = nil
	}
	return changed
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
		return profilerName == profilerBlackfire
	case fieldTidewaysAPIKey:
		return profilerName == profilerTideways
	case fieldAppEnv, fieldPHPVersion, fieldProfiler, fieldSave, fieldCount:
		return true
	}
	return true
}

func (m ConfigModel) ApplyToConfig(cfg *shop.Config) {
	if cfg.Docker == nil {
		cfg.Docker = &shop.ConfigDocker{}
	}
	if cfg.Docker.PHP == nil {
		cfg.Docker.PHP = &shop.ConfigDockerPHP{}
	}

	cfg.Docker.PHP.Version = phpVersions[m.phpVersion]
	cfg.Docker.PHP.Profiler = profilers[m.profiler]

	cfg.Docker.PHP.BlackfireServerID = ""
	cfg.Docker.PHP.BlackfireServerToken = ""
	cfg.Docker.PHP.TidewaysAPIKey = ""
}

// AppEnv returns the currently selected APP_ENV value.
func (m ConfigModel) AppEnv() string {
	return appEnvs[m.appEnv]
}

// AppEnvChanged reports whether the APP_ENV selection differs from the
// value loaded into the model.
func (m ConfigModel) AppEnvChanged() bool {
	return appEnvs[m.appEnv] != m.originalAppEnv
}

// MarkAppEnvPersisted records the current APP_ENV value as the new baseline
// after it has been written to disk.
func (m *ConfigModel) MarkAppEnvPersisted() {
	m.originalAppEnv = appEnvs[m.appEnv]
}

func (m ConfigModel) LocalConfig() *shop.Config {
	php := &shop.ConfigDockerPHP{}

	switch profilers[m.profiler] {
	case profilerBlackfire:
		php.BlackfireServerID = m.blackfireServerID.Value()
		php.BlackfireServerToken = m.blackfireServerToken.Value()
	case profilerTideways:
		php.TidewaysAPIKey = m.tidewaysAPIKey.Value()
	default:
		return nil
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

	s.WriteString(tui.TitleStyle.Render("Environment"))
	s.WriteString("\n")
	s.WriteString(m.renderSelect(fieldAppEnv, "APP_ENV", appEnvs[m.appEnv], selectedArrow, normalIndent))
	s.WriteString(divider)

	s.WriteString(tui.TitleStyle.Render("PHP"))
	s.WriteString("\n")
	s.WriteString(m.renderSelect(fieldPHPVersion, "Version", phpVersions[m.phpVersion], selectedArrow, normalIndent))
	s.WriteString(m.renderSelect(fieldProfiler, "Profiler", m.profilerLabel(), selectedArrow, normalIndent))

	profilerName := profilers[m.profiler]
	if profilerName == profilerBlackfire {
		s.WriteString(m.renderInput(fieldBlackfireServerID, "Server ID", m.blackfireServerID, selectedArrow, normalIndent))
		s.WriteString(m.renderInput(fieldBlackfireServerToken, "Server Token", m.blackfireServerToken, selectedArrow, normalIndent))
	}
	if profilerName == profilerTideways {
		s.WriteString(m.renderInput(fieldTidewaysAPIKey, "API Key", m.tidewaysAPIKey, selectedArrow, normalIndent))
	}

	s.WriteString(divider)

	if m.cursor == fieldSave {
		s.WriteString(selectedArrow)
	} else {
		s.WriteString(normalIndent)
	}
	switch {
	case m.err != nil && m.cursor == fieldSave:
		s.WriteString(activeBtnStyle.Render("Retry Save"))
	case m.err != nil:
		s.WriteString(warningBadgeStyle.Render("save failed"))
		s.WriteString("  ")
		s.WriteString(helpStyle.Render("Navigate to Retry Save"))
	case m.saved:
		s.WriteString(activeBadgeStyle.Render("Saved"))
		s.WriteString("  ")
		s.WriteString(helpStyle.Render("Restart Docker to apply changes."))
	case m.modified && m.cursor == fieldSave:
		s.WriteString(activeBtnStyle.Render("Save & Regenerate"))
	case m.modified:
		s.WriteString(warningBadgeStyle.Render("unsaved changes"))
		s.WriteString("  ")
		s.WriteString(helpStyle.Render("Navigate to Save & Regenerate"))
	default:
		s.WriteString(helpStyle.Render("No changes"))
	}
	s.WriteString("\n")

	if m.err != nil {
		s.WriteString("  ")
		s.WriteString(errorStyle.Render("Could not write config: " + m.err.Error()))
		s.WriteString("\n")
	}

	return s.String()
}

func (m ConfigModel) profilerLabel() string {
	p := profilers[m.profiler]
	if p == "" {
		return "none"
	}
	return p
}

var configKeyStyle = lipgloss.NewStyle().Width(22).Foreground(tui.TextColor)

func (m ConfigModel) renderSelect(field configField, label, value, selectedArrow, normalIndent string) string {
	prefix := normalIndent
	if m.cursor == field {
		prefix = selectedArrow
	}

	valStyle := valueStyle
	if m.cursor == field {
		valStyle = lipgloss.NewStyle().Foreground(tui.BrandColor).Bold(true)
	}

	return prefix + configKeyStyle.Render(label) + valStyle.Render(value) + "\n"
}

func (m ConfigModel) renderInput(field configField, label string, input textinput.Model, selectedArrow, normalIndent string) string {
	prefix := normalIndent
	if m.cursor == field {
		prefix = selectedArrow
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

	return prefix + configKeyStyle.Render(label) + val + hint + "\n"
}
