package devtui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/shop"
)

func writeLockWithCore(t *testing.T, dir, phpRequire string) {
	t.Helper()
	content := `{"packages":[{"name":"shopware/core","version":"v6.6.10.0","require":{"php":"` + phpRequire + `"}}]}`
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "composer.lock"), []byte(content), 0o644))
}

func TestResolvePHPVersions_NoLockFile(t *testing.T) {
	versions, idx, constraint := resolvePHPVersions(t.TempDir())
	assert.Equal(t, []string{"8.2", "8.3", "8.4", "8.5"}, versions)
	assert.Equal(t, len(versions)-1, idx)
	assert.Empty(t, constraint)
}

func TestResolvePHPVersions_FiltersByShopwareCore(t *testing.T) {
	dir := t.TempDir()
	writeLockWithCore(t, dir, "~8.2.0 || ~8.3.0")

	versions, idx, constraint := resolvePHPVersions(dir)
	assert.Equal(t, []string{"8.2", "8.3"}, versions)
	assert.Equal(t, 1, idx) // highest compatible
	assert.Equal(t, "~8.2.0 || ~8.3.0", constraint)
}

func TestResolvePHPVersions_NoMatchingVersionsFallsBackToAll(t *testing.T) {
	dir := t.TempDir()
	writeLockWithCore(t, dir, "^9.0")

	versions, idx, constraint := resolvePHPVersions(dir)
	// Constraint matches nothing → fall back to the full list so the user
	// can still pick something.
	assert.Equal(t, []string{"8.2", "8.3", "8.4", "8.5"}, versions)
	assert.Equal(t, len(versions)-1, idx)
	assert.Equal(t, "^9.0", constraint)
}

func TestSetupGuideAdminUser_EnterOnUsernameFocusesPassword(t *testing.T) {
	sg := newSetupGuide("")
	sg.step = setupStepAdminUser
	sg.credFocus = credFocusUsername
	sg.username.Focus()

	out, _ := sg.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Equal(t, setupStepAdminUser, out.step, "should stay on the admin account step")
	assert.Equal(t, credFocusPassword, out.credFocus)
	assert.True(t, out.password.Focused())
}

func TestSetupGuideAdminUser_ShortPasswordBlocksAdvance(t *testing.T) {
	sg := newSetupGuide("")
	sg.step = setupStepAdminUser
	sg.credFocus = credFocusPassword
	sg.password.SetValue("shopwar")

	out, _ := sg.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Equal(t, setupStepAdminUser, out.step, "should stay on the admin account step")
	assert.NotEmpty(t, out.passwordErr, "should set a validation error")
}

func TestSetupGuideAdminUser_ValidPasswordAdvances(t *testing.T) {
	sg := newSetupGuide("")
	sg.step = setupStepAdminUser
	sg.credFocus = credFocusPassword
	sg.password.SetValue("shopware")

	out, _ := sg.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Equal(t, setupStepDockerPHP, out.step)
	assert.Empty(t, out.passwordErr)
}

func TestSetupGuideAdminUser_TypingClearsError(t *testing.T) {
	sg := newSetupGuide("")
	sg.step = setupStepAdminUser
	sg.credFocus = credFocusPassword
	sg.passwordErr = "password must be at least 8 characters long"

	out, _ := sg.update(tea.KeyPressMsg(tea.Key{Code: 'x', Text: "x"}))
	assert.Empty(t, out.passwordErr)
}

func TestSetupGuideAdminUser_TabNavigatesFocus(t *testing.T) {
	sg := newSetupGuide("")
	sg.step = setupStepAdminUser
	sg.credFocus = credFocusUsername

	sg, _ = sg.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	assert.Equal(t, credFocusPassword, sg.credFocus)
	assert.True(t, sg.password.Focused())

	sg, _ = sg.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	assert.Equal(t, credFocusShowPassword, sg.credFocus)
	assert.False(t, sg.password.Focused())

	// Tab past the checkbox stays on the checkbox.
	sg, _ = sg.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	assert.Equal(t, credFocusShowPassword, sg.credFocus)
}

func TestSetupGuideAdminUser_EnterOnCheckboxTogglesEcho(t *testing.T) {
	sg := newSetupGuide("")
	sg.step = setupStepAdminUser
	sg.credFocus = credFocusShowPassword

	sg, _ = sg.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Equal(t, textinput.EchoNormal, sg.password.EchoMode)
	assert.Equal(t, setupStepAdminUser, sg.step)

	sg, _ = sg.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Equal(t, textinput.EchoPassword, sg.password.EchoMode)
}

func TestSetupGuideReview_QuitButtonQuits(t *testing.T) {
	sg := newSetupGuide("")
	sg.step = setupStepReview
	sg.confirmYes = false // user selected the "Quit" button

	m := Model{
		phase:      phaseSetupGuide,
		setupGuide: sg,
		config:     &shop.Config{},
		watchers:   make(map[string]*watcherHandle),
	}

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.NotNil(t, cmd, "Enter on Quit button should yield a cmd")
	_, isQuit := cmd().(tea.QuitMsg)
	assert.True(t, isQuit, "Enter on Quit button should emit tea.QuitMsg")
}

func TestSetupGuideReview_SaveButtonDoesNotQuit(t *testing.T) {
	sg := newSetupGuide("")
	sg.step = setupStepReview
	sg.confirmYes = true // user selected "Save & start"

	m := Model{
		phase:       phaseSetupGuide,
		setupGuide:  sg,
		config:      &shop.Config{},
		projectRoot: t.TempDir(),
		watchers:    make(map[string]*watcherHandle),
	}

	updated, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	// Should have transitioned to the done step, not quit.
	if cmd != nil {
		_, isQuit := cmd().(tea.QuitMsg)
		assert.False(t, isQuit, "Save button must not quit")
	}
	assert.Equal(t, setupStepDone, updated.(Model).setupGuide.step)
}

func TestMergeLocalProfilerSecrets(t *testing.T) {
	t.Run("copies blackfire and tideways onto runtime config", func(t *testing.T) {
		dst := &shop.Config{
			Docker: &shop.ConfigDocker{
				PHP: &shop.ConfigDockerPHP{Version: "8.3", Profiler: "blackfire"},
			},
		}
		src := &shop.Config{
			Docker: &shop.ConfigDocker{
				PHP: &shop.ConfigDockerPHP{
					BlackfireServerID:    "id",
					BlackfireServerToken: "token",
					TidewaysAPIKey:       "key",
				},
			},
		}
		mergeLocalProfilerSecrets(dst, src)
		assert.Equal(t, "id", dst.Docker.PHP.BlackfireServerID)
		assert.Equal(t, "token", dst.Docker.PHP.BlackfireServerToken)
		assert.Equal(t, "key", dst.Docker.PHP.TidewaysAPIKey)
		// non-secret fields stay untouched
		assert.Equal(t, "8.3", dst.Docker.PHP.Version)
		assert.Equal(t, "blackfire", dst.Docker.PHP.Profiler)
	})

	t.Run("creates intermediate structs when missing on dst", func(t *testing.T) {
		dst := &shop.Config{}
		src := &shop.Config{
			Docker: &shop.ConfigDocker{
				PHP: &shop.ConfigDockerPHP{TidewaysAPIKey: "key"},
			},
		}
		mergeLocalProfilerSecrets(dst, src)
		assert.NotNil(t, dst.Docker)
		assert.NotNil(t, dst.Docker.PHP)
		assert.Equal(t, "key", dst.Docker.PHP.TidewaysAPIKey)
	})

	t.Run("nil src is a no-op", func(t *testing.T) {
		dst := &shop.Config{Docker: &shop.ConfigDocker{PHP: &shop.ConfigDockerPHP{Version: "8.4"}}}
		mergeLocalProfilerSecrets(dst, nil)
		assert.Equal(t, "8.4", dst.Docker.PHP.Version)
		assert.Empty(t, dst.Docker.PHP.BlackfireServerID)
	})
}

func TestEnsureDeploymentHelper_AddsWhenMissing(t *testing.T) {
	dir := t.TempDir()
	composer := `{
  "name": "shopware/production",
  "require": {
    "shopware/core": "^6.6"
  }
}`
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "composer.json"), []byte(composer), 0o644))

	changed, err := ensureDeploymentHelper(dir)
	assert.NoError(t, err)
	assert.True(t, changed)

	// Verify it was actually written
	out, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	assert.NoError(t, err)
	assert.Contains(t, string(out), `"shopware/deployment-helper": "*"`)
}

func TestEnsureDeploymentHelper_NoOpWhenAlreadyInRequire(t *testing.T) {
	dir := t.TempDir()
	composer := `{
  "name": "shopware/production",
  "require": {
    "shopware/core": "^6.6",
    "shopware/deployment-helper": "^1.0"
  }
}`
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "composer.json"), []byte(composer), 0o644))

	changed, err := ensureDeploymentHelper(dir)
	assert.NoError(t, err)
	assert.False(t, changed)

	// Existing pin must not be overwritten
	out, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	assert.NoError(t, err)
	assert.Contains(t, string(out), `"shopware/deployment-helper": "^1.0"`)
}

func TestEnsureDeploymentHelper_NoOpWhenInRequireDev(t *testing.T) {
	dir := t.TempDir()
	composer := `{
  "name": "shopware/production",
  "require": {"shopware/core": "^6.6"},
  "require-dev": {"shopware/deployment-helper": "^1.0"}
}`
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "composer.json"), []byte(composer), 0o644))

	changed, err := ensureDeploymentHelper(dir)
	assert.NoError(t, err)
	assert.False(t, changed)
}

func TestEnsureDeploymentHelper_MissingComposerJson(t *testing.T) {
	changed, err := ensureDeploymentHelper(t.TempDir())
	assert.NoError(t, err)
	assert.False(t, changed)
}

func TestResolvePHPVersions_PlatformFallback(t *testing.T) {
	dir := t.TempDir()
	content := `{"packages":[{"name":"shopware/platform","version":"v6.5.0.0","require":{"php":">=8.2"}}]}`
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "composer.lock"), []byte(content), 0o644))

	versions, _, constraint := resolvePHPVersions(dir)
	assert.Equal(t, []string{"8.2", "8.3", "8.4", "8.5"}, versions)
	assert.Equal(t, ">=8.2", constraint)
}

func TestNewSetupGuide(t *testing.T) {
	sg := newSetupGuide("")
	assert.Equal(t, setupStepWelcome, sg.step)
	// Without a composer.lock the wizard offers every supported PHP version
	// and defaults the cursor to the highest.
	assert.Equal(t, len(sg.phpVersions)-1, sg.phpCursor)
	assert.True(t, sg.confirmYes)
	assert.Equal(t, "http://127.0.0.1:8000", sg.url.Value())
	assert.Equal(t, "admin", sg.username.Value())
	assert.Equal(t, "shopware", sg.password.Value())
}

func TestSetupGuideCurrentConfig(t *testing.T) {
	sg := newSetupGuide("")
	sg.phpCursor = 2 // 8.4

	c := sg.currentConfig()
	assert.Equal(t, "http://127.0.0.1:8000", c.url)
	assert.Equal(t, "admin", c.username)
	assert.Equal(t, "shopware", c.password)
	assert.Equal(t, "8.4", c.phpVersion)
}

func TestSetupGuideApplyToConfig(t *testing.T) {
	cfg := &shop.Config{}
	sg := newSetupGuide("")
	sg.phpCursor = 2 // 8.4

	sg.applyToConfig(cfg)

	assert.Equal(t, shop.CompatibilityDevMode, cfg.CompatibilityDate)
	assert.Equal(t, "http://127.0.0.1:8000", cfg.URL)
	assert.NotNil(t, cfg.Environments)
	assert.NotNil(t, cfg.Environments["local"])
	assert.Equal(t, "docker", cfg.Environments["local"].Type)
	assert.Equal(t, "http://127.0.0.1:8000", cfg.Environments["local"].URL)
	assert.NotNil(t, cfg.Environments["local"].AdminApi)
	assert.Equal(t, "admin", cfg.Environments["local"].AdminApi.Username)
	assert.Equal(t, "shopware", cfg.Environments["local"].AdminApi.Password)
	assert.NotNil(t, cfg.Docker)
	assert.NotNil(t, cfg.Docker.PHP)
	assert.Equal(t, "8.4", cfg.Docker.PHP.Version)
	assert.Equal(t, "", cfg.Docker.PHP.Profiler)
}

func TestSetupGuideApplyToConfig_PreservesExistingURL(t *testing.T) {
	cfg := &shop.Config{URL: "https://myshop.example.com"}
	sg := newSetupGuide("")

	sg.applyToConfig(cfg)

	// Should preserve existing URL at top level
	assert.Equal(t, "https://myshop.example.com", cfg.URL)
	// But local env still uses the default
	assert.Equal(t, "http://127.0.0.1:8000", cfg.Environments["local"].URL)
}

func TestSetupGuideViewSteps(t *testing.T) {
	sg := newSetupGuide("")

	// Welcome should render without panic
	view := sg.viewContent()
	assert.Contains(t, view, "Docker")

	sg.step = setupStepAdminUser
	sg.username.Focus()
	view = sg.viewContent()
	assert.Contains(t, view, "Step")
	assert.Contains(t, view, "Choose a username")
	assert.Contains(t, view, "Choose a password")

	sg.step = setupStepDockerPHP
	view = sg.viewContent()
	assert.Contains(t, view, "PHP")

	sg.step = setupStepReview
	view = sg.viewContent()
	assert.Contains(t, view, "Review")

	sg.step = setupStepDone
	view = sg.viewContent()
	assert.Contains(t, view, "saved")
}

func TestSetupGuideWelcomeDefaultConfirmYes(t *testing.T) {
	suggest := newSetupGuide("")
	assert.True(t, suggest.confirmYes)
}

func TestSetupGuideViewDone_Success(t *testing.T) {
	sg := newSetupGuide("")
	sg.step = setupStepDone

	view := sg.viewContent()
	assert.Contains(t, view, "Setup's complete!")
	assert.Contains(t, view, "Configuration saved")
	assert.Contains(t, view, "Press Enter to start the Docker containers")
}

func TestSetupGuideViewDone_Error(t *testing.T) {
	sg := newSetupGuide("")
	sg.step = setupStepDone
	sg.err = assert.AnError

	view := sg.viewContent()
	assert.Contains(t, view, "Configuration failed")
	// The success-only chrome must not leak into the error screen.
	assert.NotContains(t, view, "Setup's complete!")
	assert.NotContains(t, view, "Press Enter to start the Docker containers")
}

func TestSetupGuideDockerPHPAdvancesToReview(t *testing.T) {
	sg := newSetupGuide("")
	sg.step = setupStepDockerPHP

	next, _ := sg.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Equal(t, setupStepReview, next.step)
}

func TestSetupGuideStepNumbering(t *testing.T) {
	sg := newSetupGuide("")
	assert.Equal(t, 3, sg.totalSteps())
	assert.Equal(t, 1, sg.stepNum(setupStepAdminUser))
	assert.Equal(t, 2, sg.stepNum(setupStepDockerPHP))
	assert.Equal(t, 3, sg.stepNum(setupStepReview))
	assert.Equal(t, 0, sg.stepNum(setupStepWelcome))
	assert.Equal(t, 0, sg.stepNum(setupStepDone))
}

func TestSetupGuideWelcome_EnterSetsStartedAt(t *testing.T) {
	sg := newSetupGuide("")
	sg.confirmYes = true

	before := time.Now()
	next, _ := sg.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	after := time.Now()

	assert.False(t, next.startedAt.IsZero(), "startedAt should be set after Enter on welcome")
	assert.False(t, next.startedAt.Before(before), "startedAt should not be before test start")
	assert.False(t, next.startedAt.After(after), "startedAt should not be after test end")
}
