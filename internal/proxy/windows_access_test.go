package proxy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/docker"
)

var baseSubdomains = []string{"", "admin-watch", "storefront-watch", "adminer", "mailer"}

func TestProxyHostnames(t *testing.T) {
	t.Parallel()

	base := ProxyHostnames("winshop.shopware.local", baseSubdomains)
	assert.Equal(t, []string{
		"winshop.shopware.local",
		"admin-watch.winshop.shopware.local",
		"storefront-watch.winshop.shopware.local",
		"adminer.winshop.shopware.local",
		"mailer.winshop.shopware.local",
	}, base)

	// The environment's routed subdomains follow the lock, so the hosts line
	// matches what compose routes: optional services only appear when their
	// packages are present.
	dir := t.TempDir()
	lock := `{"packages": [{"name": "shopware/core", "version": "6.6.0.0"}, {"name": "symfony/amqp-messenger", "version": "v7.0.0"}, {"name": "shopware/elasticsearch", "version": "6.6.0.0"}, {"name": "shopware/k8s-meta", "version": "1.0.0"}], "packages-dev": []}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "composer.lock"), []byte(lock), 0o644))
	env, err := docker.NewEnvironment(dir, docker.Options{})
	require.NoError(t, err)

	full := ProxyHostnames("winshop.shopware.local", env.RoutedSubdomains())
	assert.Contains(t, full, "queue.winshop.shopware.local")
	assert.Contains(t, full, "search.winshop.shopware.local")
	assert.Contains(t, full, "s3.winshop.shopware.local")
	assert.Contains(t, full, "storage.winshop.shopware.local")
	assert.NotContains(t, base, "queue.winshop.shopware.local")
	assert.NotContains(t, base, "s3.winshop.shopware.local")
}

func TestWSLWindowsAccessGuidance(t *testing.T) {
	t.Parallel()

	caPath := "/home/tomasz/.local/share/mkcert/rootCA.pem"
	hosts := ProxyHostnames("winshop.shopware.local", baseSubdomains)

	g := WSLWindowsAccessGuidance(caPath, hosts)

	// The copy command references the real CA path and the shared Public path.
	assert.Contains(t, g, "cp "+caPath+" /mnt/c/Users/Public/shopware-cli-rootCA.pem")
	// The trust command and its admin requirement.
	assert.Contains(t, g, `certutil -addstore -f ROOT C:\Users\Public\shopware-cli-rootCA.pem`)
	assert.Contains(t, g, "Administrator")
	// A concrete shop reads in the singular.
	assert.Contains(t, g, "To open this shop")
	// The hosts path and a single ready-to-paste line with every hostname.
	assert.Contains(t, g, `C:\Windows\System32\drivers\etc\hosts`)
	assert.Contains(t, g, "127.0.0.1 winshop.shopware.local admin-watch.winshop.shopware.local storefront-watch.winshop.shopware.local adminer.winshop.shopware.local mailer.winshop.shopware.local")

	// Without hostnames (setup outside a project) it points at proxy up instead.
	empty := WSLWindowsAccessGuidance(caPath, nil)
	assert.NotContains(t, empty, "127.0.0.1 ")
	assert.Contains(t, empty, "project proxy up")
}
