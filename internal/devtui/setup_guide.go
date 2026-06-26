package devtui

import (
	"path/filepath"
	"slices"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/shopware/shopware-cli/internal/packagist"
)

type setupStep int

const (
	setupStepWelcome setupStep = iota
	setupStepAdminUser
	setupStepDockerPHP
	setupStepReview
	setupStepDone
)

type setupGuide struct {
	step                  setupStep
	phpVersions           []string // PHP versions compatible with the project
	phpCursor             int
	confirmYes            bool
	deploymentHelperAdded bool // composer.json was updated to require shopware/deployment-helper
	url                   textinput.Model
	username              textinput.Model
	password              textinput.Model
	passwordErr           string
	credFocus             credFocus
	startedAt             time.Time

	err error
}

// setupGuideConfig holds the final configuration values chosen by the user.
// The setup wizard no longer selects a PHP profiler; it defaults to none and
// the user can enable one later in the Config tab.
type setupGuideConfig struct {
	url        string
	username   string
	password   string
	phpVersion string
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

	phpVersions, phpCursor, _ := resolvePHPVersions(projectRoot)

	return setupGuide{
		step:        setupStepWelcome,
		phpVersions: phpVersions,
		phpCursor:   phpCursor,
		confirmYes:  true,
		url:         urlInput,
		username:    usernameInput,
		password:    passwordInput,
	}
}

func (sg *setupGuide) currentConfig() setupGuideConfig {
	return setupGuideConfig{
		url:        sg.url.Value(),
		username:   sg.username.Value(),
		password:   sg.password.Value(),
		phpVersion: sg.phpVersions[sg.phpCursor],
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
	case setupStepDockerPHP:
		return sg.updateDockerPHP(msg)
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
			sg.startedAt = time.Now()
			sg.step = setupStepAdminUser
			return sg.focusAdminCred(credFocusUsername)
		}
		return *sg, tea.Quit
	}
	return *sg, nil
}

// focusAdminCred moves focus to the given element of the combined admin
// account step, clamping to the valid range and syncing the text input focus.
func (sg *setupGuide) focusAdminCred(target credFocus) (setupGuide, tea.Cmd) {
	if target < credFocusUsername {
		target = credFocusUsername
	}
	if target > credFocusShowPassword {
		target = credFocusShowPassword
	}
	sg.credFocus = target
	sg.username.Blur()
	sg.password.Blur()
	var cmd tea.Cmd
	switch target {
	case credFocusUsername:
		sg.username.Focus()
		cmd = textinput.Blink
	case credFocusPassword:
		sg.password.Focus()
		cmd = textinput.Blink
	case credFocusShowPassword:
		// The checkbox has no text input to focus.
	}
	return *sg, cmd
}

func (sg *setupGuide) updateAdminUser(msg tea.KeyPressMsg) (setupGuide, tea.Cmd) {
	switch msg.String() {
	case keyEnter:
		return sg.handleAdminUserEnter()
	case keyTab, keyDown:
		return sg.focusAdminCred(sg.credFocus + 1)
	case keyShiftTab, keyUp:
		return sg.focusAdminCred(sg.credFocus - 1)
	}
	switch sg.credFocus {
	case credFocusUsername:
		var cmd tea.Cmd
		sg.username, cmd = sg.username.Update(msg)
		return *sg, cmd
	case credFocusPassword:
		sg.passwordErr = ""
		var cmd tea.Cmd
		sg.password, cmd = sg.password.Update(msg)
		return *sg, cmd
	case credFocusShowPassword:
		// The checkbox swallows typed keys.
	}
	return *sg, nil
}

func (sg *setupGuide) handleAdminUserEnter() (setupGuide, tea.Cmd) {
	switch sg.credFocus {
	case credFocusUsername:
		// Enter on the username field advances to the password field.
		return sg.focusAdminCred(credFocusPassword)
	case credFocusShowPassword:
		if sg.password.EchoMode == textinput.EchoPassword {
			sg.password.EchoMode = textinput.EchoNormal
		} else {
			sg.password.EchoMode = textinput.EchoPassword
		}
		return *sg, nil
	case credFocusPassword:
		// Enter on the password field submits; handled below.
	}
	if err := validateAdminPassword(sg.password.Value()); err != nil {
		sg.passwordErr = err.Error()
		return *sg, nil
	}
	sg.passwordErr = ""
	sg.username.Blur()
	sg.password.Blur()
	sg.step = setupStepDockerPHP
	return *sg, nil
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
		sg.step = setupStepReview
		return *sg, nil
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
