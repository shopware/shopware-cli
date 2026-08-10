package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
