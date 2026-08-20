package proxy

import (
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/dns/dnsmessage"
)

// TestEnsureDNSContainerRunningIntegration starts the real CoreDNS container
// through the production code path and checks it resolves *.<domain> to
// 127.0.0.1. It is skipped when Docker is unavailable, matching the repo's
// other Docker-dependent tests.
func TestEnsureDNSContainerRunningIntegration(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	ctx := t.Context()
	if _, err := runDocker(ctx, "version", "--format", "{{.Server.Version}}"); err != nil {
		t.Skip("docker daemon not running")
	}
	// The CoreDNS image must already be cached: CI runs this step sandboxed with
	// no network, so pulling would fail. Skip rather than pull.
	if _, err := runDocker(ctx, "image", "inspect", DNSImage); err != nil {
		t.Skipf("%s not present locally", DNSImage)
	}

	// Isolate from any real setup by pointing the state dir at a temp location.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	require.NoError(t, EnsureDNSContainerRunning(ctx, "shopware.local"))
	t.Cleanup(func() { _ = StopDNSContainer(ctx) })

	assert.True(t, dnsContainerIsRunning(ctx), "container should be running")

	// The published port takes a moment to accept queries after the run.
	addr := "127.0.0.1:53535"
	var resp dnsmessage.Message
	var err error
	for range 50 {
		resp, err = queryDNS(ctx, addr, "my-shop.shopware.local", dnsmessage.TypeA, 500*time.Millisecond)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.NoError(t, err, "DNS container did not answer")
	require.Len(t, resp.Answers, 1)

	a, ok := resp.Answers[0].Body.(*dnsmessage.AResource)
	require.True(t, ok)
	assert.Equal(t, [4]byte{127, 0, 0, 1}, a.A)

	// Idempotent: a second call must not error or replace the running container.
	require.NoError(t, EnsureDNSContainerRunning(ctx, "shopware.local"))
	assert.True(t, dnsContainerIsRunning(ctx))
}
