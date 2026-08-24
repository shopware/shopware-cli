package proxy

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/docker"
)

func TestProxyHostnames(t *testing.T) {
	t.Parallel()

	base := ProxyHostnames("winshop.shopware.local", docker.LockFeatures{})
	assert.Equal(t, []string{
		"winshop.shopware.local",
		"admin-watch.winshop.shopware.local",
		"storefront-watch.winshop.shopware.local",
		"adminer.winshop.shopware.local",
		"mailer.winshop.shopware.local",
	}, base)

	// Optional services only appear when their packages are present.
	full := ProxyHostnames("winshop.shopware.local", docker.LockFeatures{AMQP: true, Elasticsearch: true, K8sMeta: true})
	assert.Contains(t, full, "lavinmq.winshop.shopware.local")
	assert.Contains(t, full, "opensearch.winshop.shopware.local")
	assert.Contains(t, full, "s3.winshop.shopware.local")
	assert.Contains(t, full, "rustfs.winshop.shopware.local")
	assert.NotContains(t, base, "lavinmq.winshop.shopware.local")
	assert.NotContains(t, base, "s3.winshop.shopware.local")
}

func TestWSLWindowsAccessGuidance(t *testing.T) {
	t.Parallel()

	caPath := "/home/tomasz/.local/share/mkcert/rootCA.pem"
	hosts := ProxyHostnames("winshop.shopware.local", docker.LockFeatures{})

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
