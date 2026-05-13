package devtui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/shop"
)

func TestNewSetupGuide(t *testing.T) {
	sg := newSetupGuide()
	assert.Equal(t, setupStepWelcome, sg.step)
	assert.Equal(t, 1, sg.phpCursor)      // Default to 8.3
	assert.Equal(t, 0, sg.profilerCursor) // Default to none
	assert.True(t, sg.confirmYes)
	assert.Equal(t, "http://127.0.0.1:8000", sg.url.Value())
	assert.Equal(t, "admin", sg.username.Value())
	assert.Equal(t, "shopware", sg.password.Value())
}

func TestSetupGuideCurrentConfig(t *testing.T) {
	sg := newSetupGuide()
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
	sg := newSetupGuide()
	sg.profilerCursor = 1 // xdebug

	c := sg.currentConfig()
	assert.Equal(t, "xdebug", c.profiler)
}

func TestSetupGuideApplyToConfig(t *testing.T) {
	cfg := &shop.Config{}
	sg := newSetupGuide()
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
	sg := newSetupGuide()

	sg.applyToConfig(cfg)

	// Should preserve existing URL at top level
	assert.Equal(t, "https://myshop.example.com", cfg.URL)
	// But local env still uses the default
	assert.Equal(t, "http://127.0.0.1:8000", cfg.Environments["local"].URL)
}

func TestSetupGuideLocalConfig_None(t *testing.T) {
	sg := newSetupGuide()
	sg.profilerCursor = 0 // none

	assert.Nil(t, sg.localConfig())
}

func TestSetupGuideLocalConfig_Blackfire(t *testing.T) {
	sg := newSetupGuide()
	sg.profilerCursor = 2 // blackfire
	sg.blackfireServerID.SetValue("my-id")
	sg.blackfireServerToken.SetValue("my-token")

	localCfg := sg.localConfig()
	assert.NotNil(t, localCfg)
	assert.Equal(t, "my-id", localCfg.Docker.PHP.BlackfireServerID)
	assert.Equal(t, "my-token", localCfg.Docker.PHP.BlackfireServerToken)
}

func TestSetupGuideLocalConfig_Tideways(t *testing.T) {
	sg := newSetupGuide()
	sg.profilerCursor = 3 // tideways
	sg.tidewaysAPIKey.SetValue("my-key")

	localCfg := sg.localConfig()
	assert.NotNil(t, localCfg)
	assert.Equal(t, "my-key", localCfg.Docker.PHP.TidewaysAPIKey)
}

func TestSetupGuideViewSteps(t *testing.T) {
	sg := newSetupGuide()

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
	suggest := newSetupGuide()
	assert.True(t, suggest.confirmYes)
}

func TestSetupGuideProfiler_NoneSkipsCredsStep(t *testing.T) {
	sg := newSetupGuide()
	sg.step = setupStepDockerProfiler
	sg.profilerCursor = 0 // none

	next, _ := sg.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Equal(t, setupStepReview, next.step)
	assert.False(t, next.blackfireServerID.Focused())
	assert.False(t, next.tidewaysAPIKey.Focused())
}

func TestSetupGuideProfiler_BlackfireRoutesToCredsAndFocusesID(t *testing.T) {
	sg := newSetupGuide()
	sg.step = setupStepDockerProfiler
	sg.profilerCursor = 2 // blackfire

	next, _ := sg.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Equal(t, setupStepProfilerCreds, next.step)
	assert.True(t, next.blackfireServerID.Focused())
	assert.False(t, next.blackfireServerToken.Focused())
}

func TestSetupGuideProfiler_TidewaysRoutesToCredsAndFocusesKey(t *testing.T) {
	sg := newSetupGuide()
	sg.step = setupStepDockerProfiler
	sg.profilerCursor = 3 // tideways

	next, _ := sg.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Equal(t, setupStepProfilerCreds, next.step)
	assert.True(t, next.tidewaysAPIKey.Focused())
}

func TestSetupGuideProfiler_BlackfireCredsAdvanceThroughInputs(t *testing.T) {
	sg := newSetupGuide()
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
	sg := newSetupGuide()
	sg.step = setupStepDockerProfiler
	sg.profilerCursor = 3 // tideways

	sg, _ = sg.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.True(t, sg.tidewaysAPIKey.Focused())

	sg, _ = sg.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.False(t, sg.tidewaysAPIKey.Focused())
	assert.Equal(t, setupStepReview, sg.step)
}

func TestSetupGuideViewProfilerCreds(t *testing.T) {
	sg := newSetupGuide()
	sg.step = setupStepProfilerCreds
	sg.profilerCursor = 2 // blackfire
	sg.blackfireServerID.Focus()

	view := sg.viewContent()
	assert.Contains(t, view, "Blackfire")
	assert.Contains(t, view, "Server ID")
}

func TestSetupGuideStepNumbering_NoProfilerCreds(t *testing.T) {
	sg := newSetupGuide()
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
	sg := newSetupGuide()
	sg.profilerCursor = 2 // blackfire → adds creds step
	assert.Equal(t, 6, sg.totalSteps())
	assert.Equal(t, 5, sg.stepNum(setupStepProfilerCreds))
	assert.Equal(t, 6, sg.stepNum(setupStepReview))
}

func TestProfilerNeedsCreds(t *testing.T) {
	assert.False(t, profilerNeedsCreds("none"))
	assert.False(t, profilerNeedsCreds(""))
	assert.False(t, profilerNeedsCreds("xdebug"))
	assert.False(t, profilerNeedsCreds("pcov"))
	assert.False(t, profilerNeedsCreds("spx"))
	assert.True(t, profilerNeedsCreds("blackfire"))
	assert.True(t, profilerNeedsCreds("tideways"))
}
