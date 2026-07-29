package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveLocalDomainChoice(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                                                 string
		useDocker, wantLocal, promptShown, setupDone, answer bool
		wantUseLocal, wantSetupNow                           bool
	}{
		{"prompt: yes, machine not set up → local + setup", true, true, true, false, true, true, true},
		{"prompt: setup declined → local, no setup", true, true, true, false, false, true, false},
		{"already set up → local, no setup", true, true, true, true, true, true, false},
		// --local-domain flag path (promptShown=false) must never run sudo.
		{"flag (no prompt) never triggers setup", true, true, false, false, true, true, false},
		// Local domains require Docker.
		{"docker off → no local domain, no setup", false, true, true, false, true, false, false},
		{"local domains declined", true, false, true, false, true, false, false},
	}

	for _, c := range cases {
		gotLocal, gotSetup := resolveLocalDomainChoice(c.useDocker, c.wantLocal, c.promptShown, c.setupDone, c.answer)
		assert.Equal(t, c.wantUseLocal, gotLocal, c.name+" (useLocalDomain)")
		assert.Equal(t, c.wantSetupNow, gotSetup, c.name+" (setupProxyNow)")
	}
}

func TestShopNotInstalled(t *testing.T) {
	t.Parallel()

	assert.True(t, shopNotInstalled("[ERROR] No sales channels found"))
	assert.True(t, shopNotInstalled("Base table or view not found: 1146 Table 'shop.sales_channel' doesn't exist"))
	assert.True(t, shopNotInstalled("Unknown database 'shopware'"))
	assert.False(t, shopNotInstalled("some unrelated failure"))
	assert.False(t, shopNotInstalled(""))
}

func TestProjectHasReplaceURLCommand(t *testing.T) {
	t.Parallel()

	writeLock := func(t *testing.T, coreVersion string) string {
		t.Helper()
		dir := t.TempDir()
		lock := `{"packages":[{"name":"shopware/core","version":"` + coreVersion + `"}]}`
		require.NoError(t, os.WriteFile(filepath.Join(dir, "composer.lock"), []byte(lock), 0o644))
		return dir
	}

	// sales-channel:replace:url exists from 6.7 on.
	assert.False(t, projectHasReplaceURLCommand(writeLock(t, "6.6.10.21")))
	assert.True(t, projectHasReplaceURLCommand(writeLock(t, "6.7.0.0")))
	assert.True(t, projectHasReplaceURLCommand(writeLock(t, "v6.7.12.2")))
	// Unknown / unparseable versions and a missing lock default to current (6.7+).
	assert.True(t, projectHasReplaceURLCommand(writeLock(t, "dev-trunk")))
	assert.True(t, projectHasReplaceURLCommand(t.TempDir()))
}
