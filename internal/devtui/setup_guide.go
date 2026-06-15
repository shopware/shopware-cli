package devtui

import (
	"path/filepath"
	"slices"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	dockerpkg "github.com/shopware/shopware-cli/internal/docker"
	"github.com/shopware/shopware-cli/internal/packagist"
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
	passwordErr           string
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
		if err := validateAdminPassword(sg.password.Value()); err != nil {
			sg.passwordErr = err.Error()
			return *sg, nil
		}
		sg.passwordErr = ""
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
	sg.passwordErr = ""
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
