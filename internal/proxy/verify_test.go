package proxy

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOSResolutionHint(t *testing.T) {
	t.Parallel()

	hint := osResolutionHint("shopware.local", true)

	// States what is already known, so users debug the right layer.
	assert.Contains(t, hint, "The DNS server works, but your system is not asking it")
	// Actionable steps for the two universal causes.
	assert.Contains(t, hint, "shopware-cli project proxy setup")
	assert.Contains(t, hint, "disconnect the VPN")
	// No third-party product names.
	for _, product := range []string{"Cisco", "Umbrella", "Zscaler", "WARP"} {
		assert.NotContains(t, hint, product)
	}

	if runtime.GOOS == "darwin" {
		assert.Contains(t, hint, "Bonjour")
		assert.Contains(t, hint, "--domain shopware.internal")

		// The mDNS note only applies to .local domains.
		assert.NotContains(t, osResolutionHint("dev.internal", true), "Bonjour")
	}
}

func TestOSResolutionHintWithoutWildcardSupport(t *testing.T) {
	t.Parallel()

	hint := osResolutionHint("shopware.local", false)

	// Explains WHY automatic DNS cannot work...
	assert.Contains(t, hint, "systemd-resolved")
	assert.Contains(t, hint, "does not run it")
	// ...how to FIX the root cause...
	assert.Contains(t, hint, "sudo systemctl enable --now systemd-resolved")
	assert.Contains(t, hint, "sudo apt install systemd-resolved")
	assert.Contains(t, hint, "stub-resolv.conf")
	assert.Contains(t, hint, "proxy setup\" again")
	// ...and the manual last resort with an honest warning about the fix.
	assert.Contains(t, hint, "changes how your whole system resolves DNS")
	assert.Contains(t, hint, "127.0.0.1 <shop-name>.shopware.local")
}

func TestRandomProbeHostnameIsUniqueAndInZone(t *testing.T) {
	t.Parallel()

	a := randomProbeHostname("shopware.local")
	b := randomProbeHostname("shopware.local")

	assert.True(t, strings.HasSuffix(a, ".shopware.local"))
	assert.NotEqual(t, a, b)
}

func TestIdleState(t *testing.T) {
	t.Parallel()

	for state, want := range map[string]bool{
		"":           true,
		"created":    true,
		"exited":     true,
		"running":    false,
		"restarting": false,
		"paused":     false,
		"dead":       false,
	} {
		assert.Equal(t, want, idleState(state), "state %q", state)
	}
}

func TestCrashLooping(t *testing.T) {
	t.Parallel()

	cases := []struct {
		status   string
		restarts int
		want     bool
	}{
		{"restarting", 0, true},
		{"restarting", 3, true},
		{"running", 2, true},
		{"running", 0, false},
		{"exited", 5, false},
		{"", 0, false},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, crashLooping(tc.status, tc.restarts), "status %q restarts %d", tc.status, tc.restarts)
	}
}

func TestRunChecksPendingFollowsIdleState(t *testing.T) {
	t.Parallel()

	errBroken := errors.New("broken")
	fail := func(context.Context) error { return errBroken }
	pass := func(context.Context) error { return nil }
	idleYes := func(context.Context, error) bool { return true }
	idleNo := func(context.Context, error) bool { return false }
	idleUnlessBroken := func(_ context.Context, err error) bool { return !errors.Is(err, errBroken) }

	cases := []struct {
		name        string
		checks      []verifyCheck
		wantResults int
		wantPending bool
		wantHint    string
	}{
		{
			name:        "idle container makes a failure pending and shows the start hint",
			checks:      []verifyCheck{{name: "a", run: fail, hint: "fix", idleHint: "start", idle: idleYes}, {name: "b", run: pass}},
			wantResults: 1,
			wantPending: true,
			wantHint:    "start",
		},
		{
			name:        "broken container makes the same failure real and keeps the fix hint",
			checks:      []verifyCheck{{name: "a", run: fail, hint: "fix", idleHint: "start", idle: idleNo}},
			wantResults: 1,
			wantPending: false,
			wantHint:    "fix",
		},
		{
			name:        "the idle rule sees the failure and can keep it real",
			checks:      []verifyCheck{{name: "a", run: fail, hint: "fix", idleHint: "start", idle: idleUnlessBroken}},
			wantResults: 1,
			wantPending: false,
			wantHint:    "fix",
		},
		{
			name:        "checks without an idle rule never go pending",
			checks:      []verifyCheck{{name: "a", run: pass}, {name: "b", run: fail, hint: "fix"}},
			wantResults: 2,
			wantPending: false,
			wantHint:    "fix",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			results := runChecks(t.Context(), tc.checks)
			require.Len(t, results, tc.wantResults)

			last := results[len(results)-1]
			require.Error(t, last.Err)
			assert.Equal(t, tc.wantPending, last.Pending)
			assert.Equal(t, tc.wantHint, last.Hint)
			for _, r := range results[:len(results)-1] {
				assert.NoError(t, r.Err)
				assert.Empty(t, r.Hint)
			}
		})
	}
}

func TestDNSWrongAnswerIsNeverIdle(t *testing.T) {
	t.Parallel()

	assert.False(t, dnsIdle(t.Context(), fmt.Errorf("%w (asked for probe)", errWrongAnswer)))
}
