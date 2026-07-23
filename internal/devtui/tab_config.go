package devtui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	dockerpkg "github.com/shopware/shopware-cli/internal/docker"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/tui/app"
	"github.com/shopware/shopware-cli/internal/tui/picker"
	"github.com/shopware/shopware-cli/internal/tui/textprompt"
)

type configField int

const (
	fieldAppEnv configField = iota
	fieldHTTPCache
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
)

var (
	phpVersions = shop.SupportedPHPVersions
	profilers   = dockerpkg.Profilers
)

// envFieldDef describes a Symfony .env-backed choice field rendered in the
// config tab. Adding a new entry to envFields is enough to surface another
// env variable: it gets a picker, is loaded from .env.local on startup, and
// is written back through envfile.WriteValues on save.
//
// choices holds the canonical values written to .env.local. choiceLabels is
// optional; when set, it provides display labels (in the same order as
// choices) so the row and picker can show e.g. "enabled"/"disabled" while
// still writing "1"/"0".
type envFieldDef struct {
	field        configField
	key          string
	label        string
	title        string
	help         string
	choices      []string
	choiceLabels []string
	defaultIdx   int
}

func (def envFieldDef) choiceLabel(idx int) string {
	if idx < 0 || idx >= len(def.choices) {
		return ""
	}
	if def.choiceLabels != nil && idx < len(def.choiceLabels) {
		return def.choiceLabels[idx]
	}
	return def.choices[idx]
}

var envFields = []envFieldDef{
	{
		field:      fieldAppEnv,
		key:        "APP_ENV",
		label:      "APP_ENV",
		title:      "APP_ENV",
		help:       "Shopware application environment (.env.local)",
		choices:    []string{"dev", "prod", "test"},
		defaultIdx: 0,
	},
	{
		field:        fieldHTTPCache,
		key:          "SHOPWARE_HTTP_CACHE_ENABLED",
		label:        "HTTP Cache",
		title:        "HTTP Cache",
		help:         "Toggle the Shopware HTTP cache (SHOPWARE_HTTP_CACHE_ENABLED in .env.local)",
		choices:      []string{"0", "1"},
		choiceLabels: []string{"disabled", "enabled"},
		defaultIdx:   0,
	},
}

// EnvFieldKeys returns the env variable names this tab manages, suitable for
// passing to envfile.ReadValues.
func EnvFieldKeys() []string {
	keys := make([]string, len(envFields))
	for i, def := range envFields {
		keys[i] = def.key
	}
	return keys
}

func envFieldByConfigField(f configField) (envFieldDef, bool) {
	for _, def := range envFields {
		if def.field == f {
			return def, true
		}
	}
	return envFieldDef{}, false
}

type ConfigModel struct {
	cursor configField

	envSelections map[string]int    // env key -> index in envFieldDef.choices
	envOriginals  map[string]string // env key -> value loaded from .env files

	phpVersion int
	profiler   int

	blackfireServerID    textinput.Model
	blackfireServerToken textinput.Model
	tidewaysAPIKey       textinput.Model

	saved      bool
	modified   bool
	restarting bool
	err        error

	width  int
	height int
}

// NewConfigModel constructs the Config tab. envValues maps env variable
// names (see EnvFieldKeys) to the values currently effective in the
// project's .env files; missing or empty entries fall back to each field's
// default choice.
func NewConfigModel(cfg *shop.Config, envValues map[string]string) ConfigModel {
	m := ConfigModel{
		cursor:        fieldAppEnv,
		envSelections: make(map[string]int, len(envFields)),
		envOriginals:  make(map[string]string, len(envFields)),
		phpVersion:    defaultPHPVersionIndex,
	}

	for _, def := range envFields {
		current := envValues[def.key]
		if current == "" {
			current = def.choices[def.defaultIdx]
		}
		m.envSelections[def.key] = indexOf(def.choices, current, def.defaultIdx)
		m.envOriginals[def.key] = current
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

// Update routes key presses to HandleKey and ignores other messages — the
// config tab has no background work to react to. Enter (edit/save) is handled
// one level up in the parent's updateConfigTab before HandleKey is reached.
func (m ConfigModel) Update(msg tea.Msg) (ConfigModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		return m.HandleKey(key)
	}
	return m, nil
}

func (m ConfigModel) HandleKey(msg tea.KeyPressMsg) (ConfigModel, tea.Cmd) {
	switch tui.KeyString(msg) {
	case tui.KeyUp, "k":
		m.moveCursorUp()
	case tui.KeyDown, "j":
		m.moveCursorDown()
	}

	return m, nil
}

func (m ConfigModel) PickerForCursor() app.Overlay {
	if def, ok := envFieldByConfigField(m.cursor); ok {
		items := make([]picker.Item, len(def.choices))
		for i, v := range def.choices {
			items[i] = picker.Item{Label: def.choiceLabel(i), Value: v}
		}
		return picker.New(picker.Options{Key: def.field, Title: def.title, Help: def.help, Items: items, InitialIndex: m.envSelections[def.key]})
	}
	switch m.cursor { //nolint:exhaustive
	case fieldPHPVersion:
		items := make([]picker.Item, len(phpVersions))
		for i, v := range phpVersions {
			items[i] = picker.Item{Label: v, Value: v}
		}
		return picker.New(picker.Options{Key: fieldPHPVersion, Title: "PHP Version", Items: items, InitialIndex: m.phpVersion})
	case fieldProfiler:
		items := make([]picker.Item, len(profilers))
		for i, p := range profilers {
			label := p
			detail := "free"
			switch {
			case p == "":
				label = "none"
				detail = ""
			case dockerpkg.ProfilerIsPaid(p):
				detail = "paid"
			}
			items[i] = picker.Item{Label: label, Detail: detail, Value: p}
		}
		return picker.New(picker.Options{Key: fieldProfiler, Title: "PHP Profiler", Items: items, InitialIndex: m.profiler})
	case fieldBlackfireServerID:
		return textprompt.New(textprompt.Options{Key: fieldBlackfireServerID, Title: "Blackfire Server ID", Help: "Server ID for the Blackfire profiler", Value: m.blackfireServerID.Value(), Secret: true})
	case fieldBlackfireServerToken:
		return textprompt.New(textprompt.Options{Key: fieldBlackfireServerToken, Title: "Blackfire Server Token", Help: "Server token for the Blackfire profiler", Value: m.blackfireServerToken.Value(), Secret: true})
	case fieldTidewaysAPIKey:
		return textprompt.New(textprompt.Options{Key: fieldTidewaysAPIKey, Title: "Tideways API Key", Help: "API key for Tideways", Value: m.tidewaysAPIKey.Value(), Secret: true})
	}
	return nil
}

func (m *ConfigModel) ApplyPickerValue(field configField, value string) bool {
	if def, ok := envFieldByConfigField(field); ok {
		current := m.envSelections[def.key]
		idx := indexOf(def.choices, value, current)
		if idx == current {
			return false
		}
		m.envSelections[def.key] = idx
		m.modified = true
		m.saved = false
		m.err = nil
		return true
	}

	changed := false
	switch field { //nolint:exhaustive
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
	if _, ok := envFieldByConfigField(f); ok {
		return true
	}
	profilerName := profilers[m.profiler]
	switch f {
	case fieldBlackfireServerID, fieldBlackfireServerToken:
		return profilerName == profilerBlackfire
	case fieldTidewaysAPIKey:
		return profilerName == profilerTideways
	case fieldAppEnv, fieldHTTPCache, fieldPHPVersion, fieldProfiler, fieldSave, fieldCount:
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

// EnvValue returns the currently selected value for the env field with the
// given key, or "" when no such field is registered.
func (m ConfigModel) EnvValue(key string) string {
	for _, def := range envFields {
		if def.key == key {
			return def.choices[m.envSelections[key]]
		}
	}
	return ""
}

// ChangedEnvValues returns the env entries whose selected value differs from
// the value loaded into the model.
func (m ConfigModel) ChangedEnvValues() map[string]string {
	changes := make(map[string]string)
	for _, def := range envFields {
		current := def.choices[m.envSelections[def.key]]
		if current != m.envOriginals[def.key] {
			changes[def.key] = current
		}
	}
	return changes
}

// MarkEnvValuesPersisted records the current env selections as the new
// baseline after they have been written to disk.
func (m *ConfigModel) MarkEnvValuesPersisted() {
	for _, def := range envFields {
		m.envOriginals[def.key] = def.choices[m.envSelections[def.key]]
	}
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

	if len(envFields) > 0 {
		s.WriteString(tui.TitleStyle.Render("Environment"))
		s.WriteString("\n")
		for _, def := range envFields {
			s.WriteString(m.renderSelect(def.field, def.label, def.choiceLabel(m.envSelections[def.key]), selectedArrow, normalIndent))
		}
		s.WriteString(divider)
	}

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
	case m.restarting:
		s.WriteString(warningBadgeStyle.Render("restarting docker…"))
	case m.err != nil && m.cursor == fieldSave:
		s.WriteString(tui.ActiveButtonStyle.Render("Retry Save"))
	case m.err != nil:
		s.WriteString(warningBadgeStyle.Render("save failed"))
		s.WriteString("  ")
		s.WriteString(helpStyle.Render("Navigate to Retry Save"))
	case m.saved:
		s.WriteString(activeBadgeStyle.Render("Saved"))
		s.WriteString("  ")
		s.WriteString(helpStyle.Render("Docker restarted with new config."))
	case m.modified && m.cursor == fieldSave:
		s.WriteString(tui.ActiveButtonStyle.Render("Save & Regenerate"))
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
