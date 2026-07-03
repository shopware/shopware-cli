package devtui

import (
	"context"
	"fmt"
	"image/color"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/envfile"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/packagist"
	"github.com/shopware/shopware-cli/internal/symfony"
	"github.com/shopware/shopware-cli/internal/tui"
)

type healthLevel int

const (
	healthOK       healthLevel = iota // matches the recommendation
	healthWarn                        // differs from the recommendation
	healthCritical                    // hard requirement violated (e.g. unsupported PHP)
)

// healthCheck is one row of the readonly "Setup health" report: a named check
// grouped under a section, with the current value, the recommended value, and
// how severe the mismatch is. DocsURL links the check name (via an OSC 8
// hyperlink) to the documentation explaining the recommendation.
type healthCheck struct {
	Group       string
	Name        string
	Current     string
	Recommended string
	Level       healthLevel
	DocsURL     string
}

const (
	healthGroupRuntime       = "Runtime"
	healthGroupLocalBehavior = "Local behavior"
	healthGroupDebug         = "Debug (Flow Builder)"
)

// Documentation pages the check names link to.
const (
	hostingDocsURL        = "https://developer.shopware.com/docs/guides/hosting/"
	adminWorkerDocsURL    = "https://developer.shopware.com/docs/guides/hosting/infrastructure/message-queue.html#admin-worker"
	flowBuilderLogDocsURL = "https://developer.shopware.com/docs/guides/hosting/performance/performance-tweaks.html#logging"
)

type setupHealthLoadedMsg struct {
	checks []healthCheck
}

// setupHealthTimeout bounds the PHP invocation used to read the runtime values
// so a stuck container cannot leave the report loading forever.
const setupHealthTimeout = 15 * time.Second

func loadSetupHealth(projectRoot string, exec executor.Executor) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), setupHealthTimeout)
		defer cancel()
		return setupHealthLoadedMsg{checks: collectSetupHealth(ctx, projectRoot, exec)}
	}
}

// collectSetupHealth gathers all setup-health checks. Checks whose data source
// is unavailable (environment down, no config/packages tree) are omitted rather
// than reported as failures, since the report is informational.
func collectSetupHealth(ctx context.Context, projectRoot string, exec executor.Executor) []healthCheck {
	var checks []healthCheck
	checks = append(checks, runtimeHealthChecks(ctx, projectRoot, exec)...)
	checks = append(checks, projectConfigHealthChecks(projectRoot)...)
	return checks
}

// runtimeHealthChecks reads PHP_VERSION and memory_limit from the PHP runtime
// the project actually uses (inside the container for docker environments).
func runtimeHealthChecks(ctx context.Context, projectRoot string, exec executor.Executor) []healthCheck {
	if exec == nil {
		return nil
	}

	out, err := exec.PHPCommand(ctx, "-r", `echo PHP_VERSION, "\n", ini_get('memory_limit');`).Output()
	if err != nil {
		return nil
	}

	parts := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)
	phpVersion := strings.TrimSpace(parts[0])
	memoryLimit := ""
	if len(parts) > 1 {
		memoryLimit = strings.TrimSpace(parts[1])
	}

	var checks []healthCheck
	if phpVersion != "" {
		checks = append(checks, phpVersionCheck(projectRoot, phpVersion))
	}
	if memoryLimit != "" {
		checks = append(checks, memoryLimitCheck(memoryLimit))
	}
	return checks
}

// phpVersionCheck compares the running PHP version against the `require.php`
// constraint of the installed Shopware release from composer.lock.
func phpVersionCheck(projectRoot, current string) healthCheck {
	var constraint *packagist.PHPConstraint
	if lock, err := packagist.ReadComposerLock(filepath.Join(projectRoot, "composer.lock")); err == nil {
		constraint = lock.ShopwarePHPConstraint()
	}

	level := healthOK
	recommended := constraint.String()
	if recommended == "" {
		recommended = "-"
	} else if !constraint.Check(current) {
		level = healthCritical
	}

	return healthCheck{
		Group:       healthGroupRuntime,
		Name:        "PHP version",
		Current:     current,
		Recommended: recommended,
		Level:       level,
		DocsURL:     hostingDocsURL,
	}
}

// minMemoryLimitBytes is the memory_limit Shopware recommends for web requests.
const minMemoryLimitBytes = 512 * 1024 * 1024

func memoryLimitCheck(current string) healthCheck {
	level := healthWarn
	if bytes, ok := parsePHPMemoryLimit(current); ok && (bytes < 0 || bytes >= minMemoryLimitBytes) {
		level = healthOK
	}

	return healthCheck{
		Group:       healthGroupRuntime,
		Name:        "Memory limit",
		Current:     current,
		Recommended: ">= 512M",
		Level:       level,
		DocsURL:     hostingDocsURL,
	}
}

// parsePHPMemoryLimit parses a php.ini shorthand byte value ("512M", "1G",
// "-1"). A negative value means unlimited.
func parsePHPMemoryLimit(value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}

	multiplier := int64(1)
	switch value[len(value)-1] {
	case 'K', 'k':
		multiplier = 1024
	case 'M', 'm':
		multiplier = 1024 * 1024
	case 'G', 'g':
		multiplier = 1024 * 1024 * 1024
	}
	if multiplier != 1 {
		value = value[:len(value)-1]
	}

	n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0, false
	}
	return n * multiplier, true
}

// projectConfigHealthChecks derives checks from the project's config/packages
// tree, resolved for the environment the project runs in locally.
func projectConfigHealthChecks(projectRoot string) []healthCheck {
	pc, err := symfony.NewProjectConfig(projectRoot)
	if err != nil {
		return nil
	}

	env := appEnvironment(projectRoot)

	var checks []healthCheck
	if enabled, err := pc.IsAdminWorkerEnabled(env); err == nil {
		checks = append(checks, adminWorkerCheck(enabled))
	}
	checks = append(checks, flowBuilderLogLevelCheck(pc, env))
	return checks
}

// appEnvironment returns the APP_ENV configured in the project's .env files,
// defaulting to dev, which is what the development environments run with.
func appEnvironment(projectRoot string) string {
	values, err := envfile.ReadValues(projectRoot, "APP_ENV")
	if err != nil || values["APP_ENV"] == "" {
		return "dev"
	}
	return values["APP_ENV"]
}

// adminWorkerCheck reports whether Shopware's browser-based admin worker is
// disabled. The dev environments run a dedicated worker process, so the admin
// worker should be off to avoid processing the queue twice.
func adminWorkerCheck(enabled bool) healthCheck {
	current := "disabled"
	level := healthOK
	if enabled {
		current = "enabled"
		level = healthWarn
	}

	return healthCheck{
		Group:       healthGroupLocalBehavior,
		Name:        "Admin Worker",
		Current:     current,
		Recommended: "disabled",
		Level:       level,
		DocsURL:     adminWorkerDocsURL,
	}
}

// flowBuilderLogLevelPath is the monolog handler through which Shopware's Flow
// Builder writes its business event logs. Without a configured level monolog
// records everything (DEBUG), which floods the log table on busy shops.
const flowBuilderLogLevelPath = "monolog.handlers.business_event_handler_buffer.level"

// monologLevels maps monolog level names to their severity, mirroring
// Monolog\Level. Unknown names map to 0 and are treated as below WARNING.
var monologLevels = map[string]int{
	"debug":     100,
	"info":      200,
	"notice":    250,
	"warning":   300,
	"error":     400,
	"critical":  500,
	"alert":     550,
	"emergency": 600,
}

func flowBuilderLogLevelCheck(pc *symfony.ProjectConfig, env string) healthCheck {
	current := "debug"
	if value, ok, err := pc.GetResolvedConfigValue(env, flowBuilderLogLevelPath); err == nil && ok {
		if s, isString := value.(string); isString && s != "" {
			current = s
		}
	}

	level := healthWarn
	if monologLevels[strings.ToLower(current)] >= monologLevels["warning"] {
		level = healthOK
	}

	return healthCheck{
		Group:       healthGroupDebug,
		Name:        "Flow Builder log level",
		Current:     strings.ToUpper(current),
		Recommended: "min WARNING",
		Level:       level,
		DocsURL:     flowBuilderLogDocsURL,
	}
}

func (l healthLevel) color() color.Color {
	switch l {
	case healthWarn:
		return tui.WarnColor
	case healthCritical:
		return tui.ErrorColor
	default:
		return tui.SuccessColor
	}
}

// renderSetupHealth renders the readonly "Setup health" report: a header row,
// followed by the checks grouped under bold group labels, each with a colored
// status dot in the gutter. The header, group labels, and check names share
// one column so the table reads aligned; only the dots sit left of it.
func (m OverviewModel) renderSetupHealth() string {
	var s strings.Builder

	s.WriteString(tui.TitleStyle.Render("Setup health"))
	s.WriteString("\n")

	switch {
	case m.healthLoading:
		s.WriteString("  " + tui.StatusBadge("checking", tui.BrandColor) + "\n")
		return s.String()
	case len(m.health) == 0:
		s.WriteString("  " + helpStyle.Render("No setup checks available.") + "\n")
		return s.String()
	}

	nameWidth, currentWidth := lipgloss.Width("Check"), lipgloss.Width("Current")
	for _, check := range m.health {
		nameWidth = max(nameWidth, lipgloss.Width(check.Name))
		currentWidth = max(currentWidth, lipgloss.Width(check.Current))
	}
	nameStyle := lipgloss.NewStyle().Width(nameWidth + 3)
	currentStyle := lipgloss.NewStyle().Width(currentWidth + 3)

	row := func(dot, name, current, recommended string) string {
		return fmt.Sprintf("  %s %s%s%s\n", dot, nameStyle.Render(name), currentStyle.Render(current), recommended)
	}

	dim := lipgloss.NewStyle().Foreground(tui.MutedColor)
	s.WriteString(row(" ", dim.Render("Check"), dim.Render("Current"), dim.Render("Recommended")))

	group := ""
	for _, check := range m.health {
		if check.Group != group {
			group = check.Group
			s.WriteString("\n    " + tui.TitleStyle.Render(group) + "\n")
		}
		dot := lipgloss.NewStyle().Foreground(check.Level.color()).Render("●")
		name := check.Name
		if check.DocsURL != "" {
			// The name links to the docs page explaining the recommendation.
			// The OSC 8 sequence is zero-width and does not affect alignment.
			name = tui.StyledLink(check.DocsURL, check.Name, tui.LinkStyle)
		}
		s.WriteString(row(dot, name, check.Current, check.Recommended))
	}

	return s.String()
}
