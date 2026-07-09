package devtui

import (
	"path/filepath"
	"slices"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/shopware/shopware-cli/internal/packagist"
)

type migrationStep int

const (
	migrationStepWelcome migrationStep = iota
	migrationStepAdminUser
	migrationStepDockerPHP
	migrationStepReview
	migrationStepDone
)

type migrationWizard struct {
	credentialStep

	step                  migrationStep
	phpVersions           []string // PHP versions compatible with the project
	phpCursor             int
	confirmYes            bool
	deploymentHelperAdded bool // composer.json was updated to require shopware/deployment-helper
	url                   textinput.Model
	startedAt             time.Time

	err error
}

// migrationWizardConfig holds the final configuration values chosen by the user.
// The migration wizard no longer selects a PHP profiler; it defaults to none and
// the user can enable one later in the Config tab.
type migrationWizardConfig struct {
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

func newMigrationWizard(projectRoot string) migrationWizard {
	urlInput := textinput.New()
	urlInput.Placeholder = "http://127.0.0.1:8000"
	urlInput.CharLimit = 256
	urlInput.Prompt = ""
	urlInput.SetValue("http://127.0.0.1:8000")

	phpVersions, phpCursor, _ := resolvePHPVersions(projectRoot)

	return migrationWizard{
		credentialStep: newCredentialStep("admin", "shopware"),
		step:           migrationStepWelcome,
		phpVersions:    phpVersions,
		phpCursor:      phpCursor,
		confirmYes:     true,
		url:            urlInput,
	}
}

func (sg *migrationWizard) currentConfig() migrationWizardConfig {
	return migrationWizardConfig{
		url:        sg.url.Value(),
		username:   sg.username.Value(),
		password:   sg.password.Value(),
		phpVersion: sg.phpVersions[sg.phpCursor],
	}
}

// update handles key events for the migration wizard.
// Ctrl+C is handled centrally by updateKeyPress, so individual steps
// should not handle it — they just need to handle their own navigation keys.
func (sg *migrationWizard) update(msg tea.KeyPressMsg) (migrationWizard, tea.Cmd) {
	switch sg.step {
	case migrationStepWelcome:
		return sg.updateWelcome(msg)
	case migrationStepAdminUser:
		return sg.updateAdminUser(msg)
	case migrationStepDockerPHP:
		return sg.updateDockerPHP(msg)
	case migrationStepReview:
		return sg.updateReview(msg)
	case migrationStepDone:
		// Handled by updateKeyPress to transition to Docker startup
		return *sg, nil
	}
	return *sg, nil
}

func (sg *migrationWizard) updateWelcome(msg tea.KeyPressMsg) (migrationWizard, tea.Cmd) {
	switch keyString(msg) {
	case keyLeft, "h":
		sg.confirmYes = true
	case keyRight, "l":
		sg.confirmYes = false
	case keyTab:
		sg.confirmYes = !sg.confirmYes
	case keyEnter:
		if sg.confirmYes {
			sg.startedAt = time.Now()
			sg.step = migrationStepAdminUser
			return *sg, sg.focus(credFocusUsername)
		}
		return *sg, tea.Quit
	}
	return *sg, nil
}

func (sg *migrationWizard) updateAdminUser(msg tea.KeyPressMsg) (migrationWizard, tea.Cmd) {
	switch keyString(msg) {
	case keyEnter:
		return sg.handleAdminUserEnter()
	case keyTab, keyDown:
		return *sg, sg.focus(sg.credFocus + 1)
	case keyShiftTab, keyUp:
		return *sg, sg.focus(sg.credFocus - 1)
	}
	return *sg, sg.updateInput(msg)
}

func (sg *migrationWizard) handleAdminUserEnter() (migrationWizard, tea.Cmd) {
	switch sg.credFocus {
	case credFocusUsername:
		// Enter on the username field advances to the password field.
		return *sg, sg.focus(credFocusPassword)
	case credFocusShowPassword:
		sg.toggleShowPassword()
		return *sg, nil
	case credFocusPassword:
		// Enter on the password field submits; handled below.
	}
	if !sg.validatePassword() {
		return *sg, nil
	}
	sg.blur()
	sg.step = migrationStepDockerPHP
	return *sg, nil
}

func (sg *migrationWizard) updateDockerPHP(msg tea.KeyPressMsg) (migrationWizard, tea.Cmd) {
	if keyString(msg) == keyEnter {
		sg.step = migrationStepReview
		return *sg, nil
	}
	sg.phpCursor = moveCursor(sg.phpCursor, keyString(msg), len(sg.phpVersions))
	return *sg, nil
}

func (sg *migrationWizard) updateReview(msg tea.KeyPressMsg) (migrationWizard, tea.Cmd) {
	switch keyString(msg) {
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
