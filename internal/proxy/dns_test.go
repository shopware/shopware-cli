package proxy

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/dns/dnsmessage"
)

// startTestDNSServer runs the DNS server on an ephemeral port inside the test
// process and returns its address.
func startTestDNSServer(t *testing.T) string {
	t.Helper()

	// Reserve an ephemeral UDP port for the server.
	conn, err := (&net.ListenConfig{}).ListenPacket(t.Context(), "udp", "127.0.0.1:0")
	require.NoError(t, err)
	port := conn.LocalAddr().(*net.UDPAddr).Port
	require.NoError(t, conn.Close())

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	go func() {
		_ = RunDNSServer(ctx, port, "shopware.local")
	}()

	addr := fmt.Sprintf("127.0.0.1:%d", port)

	// Wait until the server answers.
	for range 50 {
		if _, err := queryDNS(ctx, addr, "probe.shopware.local", dnsmessage.TypeA, 200*time.Millisecond); err == nil {
			return addr
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatal("DNS server did not start")
	return ""
}

func TestRunDNSServer(t *testing.T) {
	addr := startTestDNSServer(t)

	t.Run("answers A queries under the domain with 127.0.0.1", func(t *testing.T) {
		resp, err := queryDNS(t.Context(), addr, "my-shop.shopware.local", dnsmessage.TypeA, time.Second)
		require.NoError(t, err)
		assert.Equal(t, dnsmessage.RCodeSuccess, resp.RCode)
		require.Len(t, resp.Answers, 1)

		a, ok := resp.Answers[0].Body.(*dnsmessage.AResource)
		require.True(t, ok)
		assert.Equal(t, [4]byte{127, 0, 0, 1}, a.A)
	})

	t.Run("answers nested subdomains", func(t *testing.T) {
		resp, err := queryDNS(t.Context(), addr, "mailer.my-shop.shopware.local", dnsmessage.TypeA, time.Second)
		require.NoError(t, err)
		assert.Equal(t, dnsmessage.RCodeSuccess, resp.RCode)
		require.Len(t, resp.Answers, 1)
	})

	t.Run("answers the domain itself", func(t *testing.T) {
		resp, err := queryDNS(t.Context(), addr, "shopware.local", dnsmessage.TypeA, time.Second)
		require.NoError(t, err)
		assert.Equal(t, dnsmessage.RCodeSuccess, resp.RCode)
		require.Len(t, resp.Answers, 1)
	})

	t.Run("matches the domain case-insensitively", func(t *testing.T) {
		resp, err := queryDNS(t.Context(), addr, "My-Shop.SHOPWARE.local", dnsmessage.TypeA, time.Second)
		require.NoError(t, err)
		assert.Equal(t, dnsmessage.RCodeSuccess, resp.RCode)
		require.Len(t, resp.Answers, 1)
	})

	t.Run("AAAA queries get an empty NOERROR answer", func(t *testing.T) {
		resp, err := queryDNS(t.Context(), addr, "my-shop.shopware.local", dnsmessage.TypeAAAA, time.Second)
		require.NoError(t, err)
		assert.Equal(t, dnsmessage.RCodeSuccess, resp.RCode)
		assert.Empty(t, resp.Answers)
	})

	t.Run("names outside the domain get NXDOMAIN", func(t *testing.T) {
		resp, err := queryDNS(t.Context(), addr, "example.com", dnsmessage.TypeA, time.Second)
		require.NoError(t, err)
		assert.Equal(t, dnsmessage.RCodeNameError, resp.RCode)
		assert.Empty(t, resp.Answers)
	})

	t.Run("does not answer names merely containing the domain", func(t *testing.T) {
		resp, err := queryDNS(t.Context(), addr, "evil-shopware.local.example.com", dnsmessage.TypeA, time.Second)
		require.NoError(t, err)
		assert.Equal(t, dnsmessage.RCodeNameError, resp.RCode)
	})
}

func TestBuildDNSResponseDropsGarbage(t *testing.T) {
	t.Parallel()

	assert.Nil(t, buildDNSResponse([]byte("not a dns packet"), "shopware.local"))
	assert.Nil(t, buildDNSResponse(nil, "shopware.local"))
}
