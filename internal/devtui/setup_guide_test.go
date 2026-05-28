package devtui

import (
	"os"
	"path/filepath"
	"testing"

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
	assert.Equal(t, 0, sg.profilerCursor) // Default to none
	assert.True(t, sg.confirmYes)
	assert.Equal(t, "http://127.0.0.1:8000", sg.url.Value())
	assert.Equal(t, "admin", sg.username.Value())
	assert.Equal(t, "shopware", sg.password.Value())
}

func TestSetupGuideCurrentConfig(t *testing.T) {
	sg := newSetupGuide("")
	sg.phpCursor = 2      // 8.4
	sg.profilerCursor = 0 // none

	c := sg.currentConfig()
	assert.Equal(t, "http://127.0.0.1:8000", c.url)
	assert.Equal(t, "admin", c.username)
	assert.Equal(t, "shopware", c.password)
	assert.Equal(t, "8.4", c.phpVersion)
	assert.Equal(t, "", c.profiler) // none -> ""
}

func TestSetupGuideCurrentConfig_Xdebug(t *testing.T) {
	sg := newSetupGuide("")
	sg.profilerCursor = 1 // xdebug

	c := sg.currentConfig()
	assert.Equal(t, "xdebug", c.profiler)
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

func TestSetupGuideLocalConfig_None(t *testing.T) {
	sg := newSetupGuide("")
	sg.profilerCursor = 0 // none

	assert.Nil(t, sg.localConfig())
}

func TestSetupGuideLocalConfig_Blackfire(t *testing.T) {
	sg := newSetupGuide("")
	sg.profilerCursor = 2 // blackfire
	sg.blackfireServerID.SetValue("my-id")
	sg.blackfireServerToken.SetValue("my-token")

	localCfg := sg.localConfig()
	assert.NotNil(t, localCfg)
	assert.Equal(t, "my-id", localCfg.Docker.PHP.BlackfireServerID)
	assert.Equal(t, "my-token", localCfg.Docker.PHP.BlackfireServerToken)
}

func TestSetupGuideLocalConfig_Tideways(t *testing.T) {
	sg := newSetupGuide("")
	sg.profilerCursor = 3 // tideways
	sg.tidewaysAPIKey.SetValue("my-key")

	localCfg := sg.localConfig()
	assert.NotNil(t, localCfg)
	assert.Equal(t, "my-key", localCfg.Docker.PHP.TidewaysAPIKey)
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

	sg.step = setupStepAdminPassword
	sg.password.Focus()
	view = sg.viewContent()
	assert.Contains(t, view, "Password")

	sg.step = setupStepDockerPHP
	view = sg.viewContent()
	assert.Contains(t, view, "PHP")

	sg.step = setupStepDockerProfiler
	view = sg.viewContent()
	assert.Contains(t, view, "Profiler")

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

func TestSetupGuideProfiler_NoneSkipsCredsStep(t *testing.T) {
	sg := newSetupGuide("")
	sg.step = setupStepDockerProfiler
	sg.profilerCursor = 0 // none

	next, _ := sg.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Equal(t, setupStepReview, next.step)
	assert.False(t, next.blackfireServerID.Focused())
	assert.False(t, next.tidewaysAPIKey.Focused())
}

func TestSetupGuideProfiler_BlackfireRoutesToCredsAndFocusesID(t *testing.T) {
	sg := newSetupGuide("")
	sg.step = setupStepDockerProfiler
	sg.profilerCursor = 2 // blackfire

	next, _ := sg.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Equal(t, setupStepProfilerCreds, next.step)
	assert.True(t, next.blackfireServerID.Focused())
	assert.False(t, next.blackfireServerToken.Focused())
}

func TestSetupGuideProfiler_TidewaysRoutesToCredsAndFocusesKey(t *testing.T) {
	sg := newSetupGuide("")
	sg.step = setupStepDockerProfiler
	sg.profilerCursor = 3 // tideways

	next, _ := sg.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Equal(t, setupStepProfilerCreds, next.step)
	assert.True(t, next.tidewaysAPIKey.Focused())
}

func TestSetupGuideProfiler_BlackfireCredsAdvanceThroughInputs(t *testing.T) {
	sg := newSetupGuide("")
	sg.step = setupStepDockerProfiler
	sg.profilerCursor = 2 // blackfire

	// Enter on profiler step → focuses Server ID
	sg, _ = sg.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.True(t, sg.blackfireServerID.Focused())

	// Enter advances ID → Token
	sg, _ = sg.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.False(t, sg.blackfireServerID.Focused())
	assert.True(t, sg.blackfireServerToken.Focused())
	assert.Equal(t, setupStepProfilerCreds, sg.step)

	// Enter on Token advances to Review
	sg, _ = sg.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.False(t, sg.blackfireServerToken.Focused())
	assert.Equal(t, setupStepReview, sg.step)
}

func TestSetupGuideProfiler_TidewaysCredsAdvanceToReview(t *testing.T) {
	sg := newSetupGuide("")
	sg.step = setupStepDockerProfiler
	sg.profilerCursor = 3 // tideways

	sg, _ = sg.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.True(t, sg.tidewaysAPIKey.Focused())

	sg, _ = sg.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.False(t, sg.tidewaysAPIKey.Focused())
	assert.Equal(t, setupStepReview, sg.step)
}

func TestSetupGuideViewProfilerCreds(t *testing.T) {
	sg := newSetupGuide("")
	sg.step = setupStepProfilerCreds
	sg.profilerCursor = 2 // blackfire
	sg.blackfireServerID.Focus()

	view := sg.viewContent()
	assert.Contains(t, view, "Blackfire")
	assert.Contains(t, view, "Server ID")
}

func TestSetupGuideStepNumbering_NoProfilerCreds(t *testing.T) {
	sg := newSetupGuide("")
	sg.profilerCursor = 0 // none → no creds step
	assert.Equal(t, 5, sg.totalSteps())
	assert.Equal(t, 1, sg.stepNum(setupStepAdminUser))
	assert.Equal(t, 2, sg.stepNum(setupStepAdminPassword))
	assert.Equal(t, 3, sg.stepNum(setupStepDockerPHP))
	assert.Equal(t, 4, sg.stepNum(setupStepDockerProfiler))
	assert.Equal(t, 5, sg.stepNum(setupStepReview))
	assert.Equal(t, 0, sg.stepNum(setupStepWelcome))
	assert.Equal(t, 0, sg.stepNum(setupStepDone))
}

func TestSetupGuideStepNumbering_WithProfilerCreds(t *testing.T) {
	sg := newSetupGuide("")
	sg.profilerCursor = 2 // blackfire → adds creds step
	assert.Equal(t, 6, sg.totalSteps())
	assert.Equal(t, 5, sg.stepNum(setupStepProfilerCreds))
	assert.Equal(t, 6, sg.stepNum(setupStepReview))
}
