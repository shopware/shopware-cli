package proxy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteDNSCorefile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, writeDNSCorefile(dir, "shopware.local"))

	content, err := os.ReadFile(filepath.Join(dir, "dns", "Corefile"))
	require.NoError(t, err)
	got := string(content)

	// The zone is the base domain, A queries answer 127.0.0.1, and AAAA is an
	// empty NOERROR so browsers do not stall on IPv6.
	assert.Contains(t, got, "shopware.local {")
	assert.Contains(t, got, "template IN A")
	assert.Contains(t, got, "IN A 127.0.0.1")
	assert.Contains(t, got, "template IN AAAA")
	assert.Contains(t, got, "rcode NOERROR")

	info, err := os.Stat(filepath.Join(dir, "dns", "Corefile"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm(), "CoreDNS 1.11+ runs as nonroot and must be able to read the bind-mounted Corefile")
}

func TestWriteDNSCorefileCustomDomain(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, writeDNSCorefile(dir, "my.example.test"))

	content, err := os.ReadFile(filepath.Join(dir, "dns", "Corefile"))
	require.NoError(t, err)

	assert.Contains(t, string(content), "my.example.test {")
}

func TestWriteDNSCorefileRepairsRestrictivePermissions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dnsDir := filepath.Join(dir, "dns")
	require.NoError(t, os.MkdirAll(dnsDir, 0o700))

	path := filepath.Join(dnsDir, "Corefile")
	require.NoError(t, os.WriteFile(path, []byte("stale"), 0o600))

	require.NoError(t, writeDNSCorefile(dir, "shopware.local"))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm(), "an existing 0600 Corefile must be chmod'd so the nonroot container user can read it")
}
