package project

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/proxy"
)

// The ready/notStarted distinction drives two different exit-code contracts:
// proxy verify exits 0 on an idle-but-healthy machine, while proxy setup
// treats notStarted as a hard failure.
func TestRunProxyVerificationSummary(t *testing.T) {
	old := proxyVerify
	t.Cleanup(func() { proxyVerify = old })

	cases := []struct {
		name           string
		results        []proxy.CheckResult
		wantReady      bool
		wantNotStarted bool
	}{
		{
			name:      "all checks pass",
			results:   []proxy.CheckResult{{Name: "resolver"}, {Name: "traefik"}},
			wantReady: true,
		},
		{
			name: "only pending failures mean not started",
			results: []proxy.CheckResult{
				{Name: "resolver"},
				{Name: "traefik", Err: errors.New("not running"), Pending: true, Hint: "run proxy up"},
			},
			wantNotStarted: true,
		},
		{
			name: "a real failure is not just pending",
			results: []proxy.CheckResult{
				{Name: "resolver", Err: errors.New("broken"), Hint: "fix the resolver"},
				{Name: "traefik", Err: errors.New("not running"), Pending: true},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			proxyVerify = func(context.Context, string) []proxy.CheckResult { return tc.results }

			ready, notStarted := runProxyVerification(t.Context(), "shopware.local")
			assert.Equal(t, tc.wantReady, ready)
			assert.Equal(t, tc.wantNotStarted, notStarted)
		})
	}
}
