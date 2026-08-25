package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/proxy"
)

func TestProxyStatusReportsUnregisteredProject(t *testing.T) {
	isolateProxyState(t)
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())

	err := proxyStatus(cmd, t.TempDir())
	require.ErrorIs(t, err, ErrProxyNotRegistered)
}

func TestProxyStatusFindsRegisteredProject(t *testing.T) {
	isolateProxyState(t)
	root := proxy.CanonicalProjectRoot(t.TempDir())
	reg := proxy.Registry{Projects: []proxy.ProjectEntry{{ProjectRoot: root, Hostname: "shop.shopware.local"}}}
	require.NoError(t, reg.Save())

	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	require.NoError(t, proxyStatus(cmd, root))
}

func TestRunningServicesParsesContainerNames(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".env"), []byte("COMPOSE_PROJECT_NAME=myshop\n"), 0o644))
	entry := proxy.ProjectEntry{ProjectRoot: root, Hostname: "shop.shopware.local"}

	instances := []proxy.Instance{
		{Container: "myshop-web-1"},
		{Container: "myshop-adminer-1"},
		{Container: "myshop-mailer-2"},
		{Container: "otherproj-web-1"},
	}

	assert.Equal(t, []string{"adminer", "mailer", "web"}, runningServices(entry, instances))
	assert.True(t, projectIsRunning(entry, instances))

	foreignOnly := []proxy.Instance{{Container: "otherproj-web-1"}}
	assert.Empty(t, runningServices(entry, foreignOnly))
	assert.False(t, projectIsRunning(entry, foreignOnly))
}

func TestProjectLinksShowsOnlyLabeledServices(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".env"), []byte("COMPOSE_PROJECT_NAME=myshop\n"), 0o644))
	entry := proxy.ProjectEntry{ProjectRoot: root, Hostname: "shop.shopware.local"}

	instances := []proxy.Instance{
		{Container: "myshop-web-1"},
		{Container: "myshop-adminer-1"},
		{Container: "myshop-mailer-1"},
	}

	links := projectLinks(entry, instances)
	require.GreaterOrEqual(t, len(links), 2)
	assert.Equal(t, serviceLink{label: "Shop", url: "https://shop.shopware.local"}, links[0])
	assert.Equal(t, serviceLink{label: "Admin", url: "https://shop.shopware.local/admin"}, links[1])

	assert.Contains(t, links, serviceLink{label: "Adminer", url: "https://adminer.shop.shopware.local"})
	assert.Contains(t, links, serviceLink{label: "Mailpit", url: "https://mailer.shop.shopware.local"})
	// Unlabeled services like web add no link.
	assert.Len(t, links, 4)
}
