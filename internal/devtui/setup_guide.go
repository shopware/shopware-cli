package devtui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	dockerpkg "github.com/shopware/shopware-cli/internal/docker"
	"github.com/shopware/shopware-cli/internal/packagist"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/tui"
)

type setupStep int

const (
	setupStepWelcome setupStep = iota
	setupStepAdminUser
	setupStepAdminPassword
	setupStepDockerPHP
	setupStepDockerProfiler
	setupStepProfilerCreds
	setupStepReview
	setupStepDone
)

// profilerChoices is the list of profiler options shown in the setup guide UI.
// The first entry uses "none" for display; internally it maps to "" (empty string)
// which is the canonical value stored in config.
var profilerChoices = []string{"none", dockerpkg.ProfilerXdebug, dockerpkg.ProfilerBlackfire, dockerpkg.ProfilerTideways, dockerpkg.ProfilerPcov, dockerpkg.ProfilerSpx}

type setupGuide struct {
	step                  setupStep
	phpVersions           []string // PHP versions compatible with the project
	phpConstraint         string   // raw constraint string, for display ("" if none)
	phpCursor             int
	profilerCursor        int
	confirmYes            bool
	deploymentHelperAdded bool // composer.json was updated to require shopware/deployment-helper
	url                   textinput.Model
	username              textinput.Model
	password              textinput.Model
	showPassword          bool
	blackfireServerID     textinput.Model
	blackfireServerToken  textinput.Model
	tidewaysAPIKey        textinput.Model

	err error
}

// setupGuideConfig holds the final configuration values chosen by the user.
type setupGuideConfig struct {
	url                  string
	username             string
	password             string
	phpVersion           string
	profiler             string
	blackfireServerID    string
	blackfireServerToken string
	tidewaysAPIKey       string
}

// resolvePHPVersions reads composer.lock from projectRoot and returns the
// supported PHP versions that satisfy shopware/core (or shopware/platform)'s
// require.php, the index of the highest compatible version (best default),
// and the raw constraint string for display. If composer.lock is missing or
// the Shopware package declares no PHP requirement, all SupportedPHPVersions
// are returned.
func resolvePHPVersions(projectRoot string) (versions []string, defaultIdx int, constraint string) {
	versions = append([]string(nil), packagist.SupportedPHPVersions...)
	defaultIdx = len(versions) - 1

	lock, err := packagist.ReadComposerLock(filepath.Join(projectRoot, "composer.lock"))
	if err != nil {
		return versions, defaultIdx, ""
	}

	c := lock.ShopwarePHPConstraint()
	if c == nil {
		return versions, defaultIdx, ""
	}

	filtered := c.SupportedVersions()
	if len(filtered) == 0 {
		return versions, defaultIdx, c.String()
	}

	idx := slices.Index(filtered, c.HighestSupported())
	if idx < 0 {
		idx = len(filtered) - 1
	}
	return filtered, idx, c.String()
}

func newSetupGuide(projectRoot string) setupGuide {
	urlInput := textinput.New()
	urlInput.Placeholder = "http://127.0.0.1:8000"
	urlInput.CharLimit = 256
	urlInput.Prompt = ""
	urlInput.SetValue("http://127.0.0.1:8000")

	usernameInput := textinput.New()
	usernameInput.Placeholder = "admin"
	usernameInput.CharLimit = 64
	usernameInput.Prompt = ""
	usernameInput.SetValue("admin")

	passwordInput := textinput.New()
	passwordInput.Placeholder = "shopware"
	passwordInput.CharLimit = 64
	passwordInput.Prompt = ""
	passwordInput.EchoMode = textinput.EchoPassword
	passwordInput.SetValue("shopware")

	blackfireIDInput := textinput.New()
	blackfireIDInput.Placeholder = "Server ID"
	blackfireIDInput.CharLimit = 128
	blackfireIDInput.Prompt = ""

	blackfireTokenInput := textinput.New()
	blackfireTokenInput.Placeholder = "Server Token"
	blackfireTokenInput.CharLimit = 128
	blackfireTokenInput.Prompt = ""

	tidewaysKeyInput := textinput.New()
	tidewaysKeyInput.Placeholder = "API Key"
	tidewaysKeyInput.CharLimit = 128
	tidewaysKeyInput.Prompt = ""

	phpVersions, phpCursor, phpConstraint := resolvePHPVersions(projectRoot)

	return setupGuide{
		step:                 setupStepWelcome,
		phpVersions:          phpVersions,
		phpConstraint:        phpConstraint,
		phpCursor:            phpCursor,
		profilerCursor:       0, // Default to none
		confirmYes:           true,
		url:                  urlInput,
		username:             usernameInput,
		password:             passwordInput,
		blackfireServerID:    blackfireIDInput,
		blackfireServerToken: blackfireTokenInput,
		tidewaysAPIKey:       tidewaysKeyInput,
	}
}

func (sg *setupGuide) currentConfig() setupGuideConfig {
	profiler := profilerChoices[sg.profilerCursor]
	if profiler == "none" {
		profiler = ""
	}
	return setupGuideConfig{
		url:                  sg.url.Value(),
		username:             sg.username.Value(),
		password:             sg.password.Value(),
		phpVersion:           sg.phpVersions[sg.phpCursor],
		profiler:             profiler,
		blackfireServerID:    sg.blackfireServerID.Value(),
		blackfireServerToken: sg.blackfireServerToken.Value(),
		tidewaysAPIKey:       sg.tidewaysAPIKey.Value(),
	}
}

func (sg *setupGuide) applyToConfig(cfg *shop.Config) {
	c := sg.currentConfig()

	// Always update compatibility_date to support dev mode
	cfg.CompatibilityDate = shop.CompatibilityDevMode

	// Set URL at top level for backwards compatibility
	if cfg.URL == "" {
		cfg.URL = c.url
	}

	// Set up local environment as Docker
	envCfg := &shop.EnvironmentConfig{
		Type: "docker",
		URL:  c.url,
	}
	if c.username != "" || c.password != "" {
		envCfg.AdminApi = &shop.ConfigAdminApi{
			Username: c.username,
			Password: c.password,
		}
	}
	if cfg.Environments == nil {
		cfg.Environments = make(map[string]*shop.EnvironmentConfig)
	}
	cfg.Environments["local"] = envCfg

	// Set Docker config
	if cfg.Docker == nil {
		cfg.Docker = &shop.ConfigDocker{}
	}
	if cfg.Docker.PHP == nil {
		cfg.Docker.PHP = &shop.ConfigDockerPHP{}
	}
	cfg.Docker.PHP.Version = c.phpVersion
	cfg.Docker.PHP.Profiler = c.profiler
}

// ensureDeploymentHelper adds shopware/deployment-helper to the project's
// composer.json require block when it's missing. New projects created via
// `shopware-cli project create` pin this package; older projects being
// migrated to dev mode need it added so devtui can run
// `vendor/bin/shopware-deployment-helper`.
//
// Returns true when composer.json was changed and the user should re-run
// `composer install` (or `composer update`) to pull the package in.
// Errors reading or writing composer.json are returned to the caller;
// a missing composer.json is treated as nothing-to-do (returns false, nil).
func ensureDeploymentHelper(projectRoot string) (changed bool, err error) {
	composerPath := filepath.Join(projectRoot, "composer.json")
	if _, statErr := os.Stat(composerPath); statErr != nil {
		if os.IsNotExist(statErr) {
			return false, nil
		}
		return false, statErr
	}

	cj, err := packagist.ReadComposerJson(composerPath)
	if err != nil {
		return false, err
	}

	if cj.HasPackage("shopware/deployment-helper") || cj.HasPackageDev("shopware/deployment-helper") {
		return false, nil
	}

	if cj.Require == nil {
		cj.Require = packagist.ComposerPackageLink{}
	}
	cj.Require["shopware/deployment-helper"] = "*"

	if err := cj.Save(); err != nil {
		return false, err
	}
	return true, nil
}

// localConfig returns a partial Config containing secrets that should be
// written to .shopware-project.local.yml. Profilers without external
// credentials (none, xdebug, pcov, spx) return nil.
func (sg *setupGuide) localConfig() *shop.Config {
	c := sg.currentConfig()
	switch c.profiler {
	case "blackfire":
		return &shop.Config{
			Docker: &shop.ConfigDocker{
				PHP: &shop.ConfigDockerPHP{
					BlackfireServerID:    c.blackfireServerID,
					BlackfireServerToken: c.blackfireServerToken,
				},
			},
		}
	case "tideways":
		return &shop.Config{
			Docker: &shop.ConfigDocker{
				PHP: &shop.ConfigDockerPHP{
					TidewaysAPIKey: c.tidewaysAPIKey,
				},
			},
		}
	}
	return nil
}

// update handles key events for the setup guide.
// Ctrl+C is handled centrally by updateKeyPress, so individual steps
// should not handle it — they just need to handle their own navigation keys.
func (sg *setupGuide) update(msg tea.KeyPressMsg) (setupGuide, tea.Cmd) {
	switch sg.step {
	case setupStepWelcome:
		return sg.updateWelcome(msg)
	case setupStepAdminUser:
		return sg.updateAdminUser(msg)
	case setupStepAdminPassword:
		return sg.updateAdminPassword(msg)
	case setupStepDockerPHP:
		return sg.updateDockerPHP(msg)
	case setupStepDockerProfiler:
		return sg.updateDockerProfiler(msg)
	case setupStepProfilerCreds:
		return sg.updateProfilerCreds(msg)
	case setupStepReview:
		return sg.updateReview(msg)
	case setupStepDone:
		// Handled by updateKeyPress to transition to Docker startup
		return *sg, nil
	}
	return *sg, nil
}

func (sg *setupGuide) updateWelcome(msg tea.KeyPressMsg) (setupGuide, tea.Cmd) {
	switch msg.String() {
	case keyLeft, "h":
		sg.confirmYes = true
	case keyRight, "l":
		sg.confirmYes = false
	case keyTab:
		sg.confirmYes = !sg.confirmYes
	case keyEnter:
		if sg.confirmYes {
			sg.step = setupStepAdminUser
			sg.username.Focus()
			return *sg, textinput.Blink
		}
		return *sg, tea.Quit
	}
	return *sg, nil
}

func (sg *setupGuide) updateAdminUser(msg tea.KeyPressMsg) (setupGuide, tea.Cmd) {
	if msg.String() == keyEnter {
		sg.username.Blur()
		sg.step = setupStepAdminPassword
		sg.password.Focus()
		return *sg, textinput.Blink
	}
	var cmd tea.Cmd
	sg.username, cmd = sg.username.Update(msg)
	return *sg, cmd
}

func (sg *setupGuide) updateAdminPassword(msg tea.KeyPressMsg) (setupGuide, tea.Cmd) {
	switch msg.String() {
	case keyEnter:
		sg.password.Blur()
		sg.step = setupStepDockerPHP
		return *sg, nil
	case keyTab:
		sg.showPassword = !sg.showPassword
		if sg.showPassword {
			sg.password.EchoMode = textinput.EchoNormal
		} else {
			sg.password.EchoMode = textinput.EchoPassword
		}
		return *sg, nil
	}
	var cmd tea.Cmd
	sg.password, cmd = sg.password.Update(msg)
	return *sg, cmd
}

func (sg *setupGuide) updateDockerPHP(msg tea.KeyPressMsg) (setupGuide, tea.Cmd) {
	switch msg.String() {
	case keyUp, keyK:
		if sg.phpCursor > 0 {
			sg.phpCursor--
		}
	case keyDown, keyJ:
		if sg.phpCursor < len(sg.phpVersions)-1 {
			sg.phpCursor++
		}
	case keyEnter:
		sg.step = setupStepDockerProfiler
		return *sg, nil
	}
	return *sg, nil
}

func (sg *setupGuide) updateDockerProfiler(msg tea.KeyPressMsg) (setupGuide, tea.Cmd) {
	switch msg.String() {
	case keyUp, keyK:
		if sg.profilerCursor > 0 {
			sg.profilerCursor--
		}
	case keyDown, keyJ:
		if sg.profilerCursor < len(profilerChoices)-1 {
			sg.profilerCursor++
		}
	case keyEnter:
		profiler := profilerChoices[sg.profilerCursor]
		if !dockerpkg.ProfilerNeedsCredentials(profiler) {
			sg.step = setupStepReview
			return *sg, nil
		}
		sg.step = setupStepProfilerCreds
		switch profiler {
		case "blackfire":
			sg.blackfireServerID.Focus()
		case "tideways":
			sg.tidewaysAPIKey.Focus()
		}
		return *sg, textinput.Blink
	}
	return *sg, nil
}

func (sg *setupGuide) updateProfilerCreds(msg tea.KeyPressMsg) (setupGuide, tea.Cmd) {
	if sg.blackfireServerID.Focused() {
		if msg.String() == keyEnter {
			sg.blackfireServerID.Blur()
			sg.blackfireServerToken.Focus()
			return *sg, textinput.Blink
		}
		var cmd tea.Cmd
		sg.blackfireServerID, cmd = sg.blackfireServerID.Update(msg)
		return *sg, cmd
	}

	if sg.blackfireServerToken.Focused() {
		if msg.String() == keyEnter {
			sg.blackfireServerToken.Blur()
			sg.step = setupStepReview
			return *sg, nil
		}
		var cmd tea.Cmd
		sg.blackfireServerToken, cmd = sg.blackfireServerToken.Update(msg)
		return *sg, cmd
	}

	if sg.tidewaysAPIKey.Focused() {
		if msg.String() == keyEnter {
			sg.tidewaysAPIKey.Blur()
			sg.step = setupStepReview
			return *sg, nil
		}
		var cmd tea.Cmd
		sg.tidewaysAPIKey, cmd = sg.tidewaysAPIKey.Update(msg)
		return *sg, cmd
	}

	return *sg, nil
}

func (sg *setupGuide) updateReview(msg tea.KeyPressMsg) (setupGuide, tea.Cmd) {
	switch msg.String() {
	case keyLeft, "h":
		sg.confirmYes = true
	case keyRight, "l":
		sg.confirmYes = false
	case keyTab:
		sg.confirmYes = !sg.confirmYes
	case keyEnter:
		return *sg, nil // Handled by updateKeyPress to trigger write
	}
	return *sg, nil
}

func (sg setupGuide) viewContent() string {
	switch sg.step {
	case setupStepWelcome:
		return sg.viewWelcome()
	case setupStepAdminUser:
		return sg.viewAdminUser()
	case setupStepAdminPassword:
		return sg.viewAdminPassword()
	case setupStepDockerPHP:
		return sg.viewDockerPHP()
	case setupStepDockerProfiler:
		return sg.viewDockerProfiler()
	case setupStepProfilerCreds:
		return sg.viewProfilerCreds()
	case setupStepReview:
		return sg.viewReview()
	case setupStepDone:
		return sg.viewDone()
	}
	return ""
}

func stepBadge(stepNum, totalSteps int) string {
	return tui.TextBadge(fmt.Sprintf("Step %d/%d", stepNum, totalSteps))
}

// totalSteps returns the number of numbered wizard steps. The profiler
// credentials step is only counted when the chosen profiler needs them.
func (sg setupGuide) totalSteps() int {
	// admin user, admin password, PHP version, profiler, review
	total := 5
	if dockerpkg.ProfilerNeedsCredentials(profilerChoices[sg.profilerCursor]) {
		total++
	}
	return total
}

// stepNum returns the 1-based index of the given wizard step within the
// currently active step sequence. Steps outside the numbered sequence
// (welcome, done) return 0.
func (sg setupGuide) stepNum(step setupStep) int {
	switch step {
	case setupStepAdminUser:
		return 1
	case setupStepAdminPassword:
		return 2
	case setupStepDockerPHP:
		return 3
	case setupStepDockerProfiler:
		return 4
	case setupStepProfilerCreds:
		return 5
	case setupStepReview:
		if dockerpkg.ProfilerNeedsCredentials(profilerChoices[sg.profilerCursor]) {
			return 6
		}
		return 5
	case setupStepWelcome, setupStepDone:
		return 0
	}
	return 0
}

func (sg setupGuide) viewWelcome() string {
	var b strings.Builder
	b.WriteString(tui.TextBadge("Setup"))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(tui.BrandColor).Render("Set up Docker development environment"))
	b.WriteString("\n\n")
	b.WriteString(tui.DimStyle.Render("This project needs a development environment configuration"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("before you can use "))
	b.WriteString(tui.BoldText.Render("shopware-cli project dev"))
	b.WriteString(tui.DimStyle.Render("."))
	b.WriteString("\n\n")
	b.WriteString(tui.DimStyle.Render("The setup will create a Docker-based local environment"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("with the following services:"))
	b.WriteString("\n\n")
	b.WriteString(tui.DimStyle.Render("  • ") + valueStyle.Render("Shopware") + tui.DimStyle.Render(" — your shop at http://127.0.0.1:8000"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("  • ") + valueStyle.Render("Adminer") + tui.DimStyle.Render(" — database GUI"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("  • ") + valueStyle.Render("Mailpit") + tui.DimStyle.Render(" — email testing"))
	b.WriteString("\n\n")
	b.WriteString(tui.DimStyle.Render("This will create a "))
	b.WriteString(tui.BoldText.Render(".shopware-project.yml"))
	b.WriteString(tui.DimStyle.Render(" configuration file."))
	b.WriteString("\n\n")
	b.WriteString(renderConfirmButtons("Start setup", "Quit", sg.confirmYes))
	b.WriteString("\n\n")
	return tui.RenderPhaseCardCowsay("Let me help you to set up Docker!", b.String())
}

func (sg setupGuide) viewAdminUser() string {
	var b strings.Builder
	b.WriteString(stepBadge(sg.stepNum(setupStepAdminUser), sg.totalSteps()))
	b.WriteString("\n\n")
	b.WriteString(tui.TitleStyle.Render("Admin Username"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("Username for the Shopware admin panel and API access."))
	b.WriteString("\n\n")
	b.WriteString(sg.username.View())
	b.WriteString("\n\n")
	return tui.RenderPhaseCard(b.String())
}

func (sg setupGuide) viewAdminPassword() string {
	var b strings.Builder
	b.WriteString(stepBadge(sg.stepNum(setupStepAdminPassword), sg.totalSteps()))
	b.WriteString("\n\n")
	b.WriteString(tui.TitleStyle.Render("Admin Password"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("Password for the Shopware admin panel and API access."))
	b.WriteString("\n\n")
	b.WriteString(sg.password.View())
	b.WriteString("\n\n")
	b.WriteString(renderShowPasswordCheckbox(sg.showPassword, false))
	b.WriteString("\n\n")
	return tui.RenderPhaseCard(b.String())
}

func (sg setupGuide) viewDockerPHP() string {
	var b strings.Builder
	b.WriteString(stepBadge(sg.stepNum(setupStepDockerPHP), sg.totalSteps()))
	b.WriteString("\n\n")
	b.WriteString(tui.TitleStyle.Render("Docker Configuration"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("Select the PHP version for your Docker containers."))
	if sg.phpConstraint != "" {
		b.WriteString("\n")
		b.WriteString(tui.DimStyle.Render("Filtered by shopware/core require.php: "))
		b.WriteString(valueStyle.Render(sg.phpConstraint))
	}
	b.WriteString("\n\n")

	opts := make([]tui.SelectOption, len(sg.phpVersions))
	for i, v := range sg.phpVersions {
		opts[i] = tui.SelectOption{Label: "PHP " + v}
	}
	b.WriteString(tui.RenderSelectList("PHP Version", "", opts, sg.phpCursor))
	b.WriteString("\n")
	return tui.RenderPhaseCard(b.String())
}

func (sg setupGuide) viewDockerProfiler() string {
	var b strings.Builder
	b.WriteString(stepBadge(sg.stepNum(setupStepDockerProfiler), sg.totalSteps()))
	b.WriteString("\n\n")
	b.WriteString(tui.TitleStyle.Render("PHP Profiler"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("Optionally enable a profiler for debugging."))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("Can be changed later in the Config tab."))
	b.WriteString("\n\n")

	opts := make([]tui.SelectOption, len(profilerChoices))
	for i, p := range profilerChoices {
		label := p
		desc := ""
		switch p {
		case "none":
			label = "None"
			desc = "recommended"
		case "xdebug":
			desc = "step debugging"
		}
		opts[i] = tui.SelectOption{Label: label, Detail: desc}
	}
	b.WriteString(tui.RenderSelectList("Profiler", "", opts, sg.profilerCursor))
	b.WriteString("\n")
	return tui.RenderPhaseCard(b.String())
}

func (sg setupGuide) viewProfilerCreds() string {
	var b strings.Builder
	b.WriteString(stepBadge(sg.stepNum(setupStepProfilerCreds), sg.totalSteps()))
	b.WriteString("\n\n")

	switch {
	case sg.blackfireServerID.Focused():
		b.WriteString(tui.TitleStyle.Render("Blackfire Configuration"))
		b.WriteString("\n")
		b.WriteString(tui.DimStyle.Render("Enter your Blackfire Server ID from your Blackfire account."))
		b.WriteString("\n\n")
		b.WriteString(sg.blackfireServerID.View())
	case sg.blackfireServerToken.Focused():
		b.WriteString(tui.TitleStyle.Render("Blackfire Configuration"))
		b.WriteString("\n")
		b.WriteString(tui.DimStyle.Render("Enter your Blackfire Server Token."))
		b.WriteString("\n\n")
		b.WriteString(sg.blackfireServerToken.View())
	case sg.tidewaysAPIKey.Focused():
		b.WriteString(tui.TitleStyle.Render("Tideways Configuration"))
		b.WriteString("\n")
		b.WriteString(tui.DimStyle.Render("Enter your Tideways API key."))
		b.WriteString("\n\n")
		b.WriteString(sg.tidewaysAPIKey.View())
	}

	b.WriteString("\n\n")
	return tui.RenderPhaseCard(b.String())
}

func (sg setupGuide) viewReview() string {
	c := sg.currentConfig()
	var b strings.Builder
	b.WriteString(stepBadge(sg.stepNum(setupStepReview), sg.totalSteps()))
	b.WriteString("\n\n")
	b.WriteString(tui.TitleStyle.Render("Review Configuration"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("The following configuration will be written."))
	b.WriteString("\n\n")

	divider := tui.SectionDivider(60)
	b.WriteString(tui.KVRow("Environment", activeBadgeStyle.Render("Docker")))
	b.WriteString(tui.KVRow("Shop URL", urlStyle.Render(c.url)))
	b.WriteString(tui.KVRow("Username", valueStyle.Render(c.username)))
	b.WriteString(tui.KVRow("Password", secretStyle.Render(strings.Repeat("•", len(c.password)))))
	b.WriteString(divider)
	b.WriteString(tui.KVRow("PHP Version", valueStyle.Render("PHP "+c.phpVersion)))
	profilerLabel := c.profiler
	if profilerLabel == "" {
		profilerLabel = "none"
	}
	b.WriteString(tui.KVRow("Profiler", valueStyle.Render(profilerLabel)))

	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("This will create:"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("  • ") + tui.BoldText.Render(".shopware-project.yml"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("  • ") + tui.BoldText.Render("compose.yaml"))
	b.WriteString("\n")

	if c.profiler == "blackfire" || c.profiler == "tideways" {
		b.WriteString(tui.DimStyle.Render("  • ") + tui.BoldText.Render(".shopware-project.local.yml"))
		b.WriteString(tui.DimStyle.Render(" (secrets only)"))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(renderConfirmButtons("Save & start", "Quit", sg.confirmYes))
	b.WriteString("\n\n")

	return tui.RenderPhaseCard(b.String())
}

func (sg setupGuide) viewDone() string {
	var b strings.Builder
	if sg.err != nil {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(tui.ErrorColor).Render("Configuration failed"))
		b.WriteString("\n\n")
		b.WriteString(errorStyle.Render(sg.err.Error()))
		b.WriteString("\n\n")
		b.WriteString(tui.DimStyle.Render("You can manually create ") + tui.BoldText.Render(".shopware-project.yml"))
		b.WriteString("\n")
		b.WriteString(tui.DimStyle.Render("or try again with shopware-cli project dev"))
	} else {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(tui.SuccessColor).Render("✓ Configuration saved"))
		b.WriteString("\n\n")
		b.WriteString(tui.DimStyle.Render("Your project is now configured for Docker development."))
		b.WriteString("\n")
		b.WriteString(tui.DimStyle.Render("The environment will start on the next screen."))

		if sg.deploymentHelperAdded {
			b.WriteString("\n\n")
			b.WriteString(tui.BoldText.Render("Note: "))
			b.WriteString(tui.DimStyle.Render("Added "))
			b.WriteString(valueStyle.Render("shopware/deployment-helper"))
			b.WriteString(tui.DimStyle.Render(" to "))
			b.WriteString(tui.BoldText.Render("composer.json"))
			b.WriteString(tui.DimStyle.Render("."))
			b.WriteString("\n")
			b.WriteString(tui.DimStyle.Render("Run "))
			b.WriteString(tui.BoldText.Render("composer update shopware/deployment-helper"))
			b.WriteString(tui.DimStyle.Render(" before installing Shopware."))
		}
	}
	b.WriteString("\n\n")
	return tui.RenderPhaseCard(b.String())
}

func (sg setupGuide) footerHint() string {
	// phaseHeaderFooter always appends a "ctrl+c Exit" badge, so don't
	// repeat it here.
	switch sg.step {
	case setupStepWelcome:
		return tui.ShortcutBar(
			tui.Shortcut{Key: "←/→", Label: "Select"},
			tui.Shortcut{Key: "enter", Label: "Confirm"},
		)
	case setupStepAdminUser:
		return tui.ShortcutBar(
			tui.Shortcut{Key: "enter", Label: "Continue"},
		)
	case setupStepAdminPassword:
		return tui.ShortcutBar(
			tui.Shortcut{Key: "tab", Label: "Show password"},
			tui.Shortcut{Key: "enter", Label: "Continue"},
		)
	case setupStepDockerPHP, setupStepDockerProfiler:
		return tui.ShortcutBar(
			tui.Shortcut{Key: "↑/↓", Label: "Select"},
			tui.Shortcut{Key: "enter", Label: "Continue"},
		)
	case setupStepProfilerCreds:
		return tui.ShortcutBar(
			tui.Shortcut{Key: "enter", Label: "Continue"},
		)
	case setupStepReview:
		return tui.ShortcutBar(
			tui.Shortcut{Key: "←/→", Label: "Select"},
			tui.Shortcut{Key: "enter", Label: "Confirm"},
		)
	case setupStepDone:
		return tui.ShortcutBar(tui.Shortcut{Key: "enter", Label: "Continue"})
	}
	return ""
}
