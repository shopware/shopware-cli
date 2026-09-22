package verifier

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShopwarePackageEnvName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "SHOPWARE_CORE_ROOT", shopwarePackageEnvName("core"))
	assert.Equal(t, "SHOPWARE_ADMIN_ROOT", shopwarePackageEnvName("administration"))
	assert.Equal(t, "SHOPWARE_STOREFRONT_ROOT", shopwarePackageEnvName("storefront"))
	assert.Equal(t, "SHOPWARE_ELASTICSEARCH_ROOT", shopwarePackageEnvName("elasticsearch"))
	assert.Equal(t, "SHOPWARE_DEV_TOOLS_ROOT", shopwarePackageEnvName("dev-tools"))
}

func TestShopwarePackageRootsFromVendor(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	core := filepath.Join(root, "vendor", "shopware", "core")
	admin := filepath.Join(root, "vendor", "shopware", "administration")
	require.NoError(t, os.MkdirAll(core, 0o755))
	require.NoError(t, os.MkdirAll(admin, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "vendor", "shopware", "conflicts"), []byte(""), 0o644))

	plugin := filepath.Join(root, "custom", "plugins", "Demo")
	require.NoError(t, os.MkdirAll(plugin, 0o755))

	got := shopwarePackageRoots(plugin)

	assert.Equal(t, map[string]string{
		"SHOPWARE_ADMIN_ROOT": admin,
		"SHOPWARE_CORE_ROOT":  core,
	}, got)
}

func TestShopwarePackageRootsPrefersNearestInstall(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	projectCore := filepath.Join(root, "vendor", "shopware", "core")
	plugin := filepath.Join(root, "custom", "plugins", "Demo")
	pluginCore := filepath.Join(plugin, "vendor", "shopware", "core")
	require.NoError(t, os.MkdirAll(projectCore, 0o755))
	require.NoError(t, os.MkdirAll(pluginCore, 0o755))

	got := shopwarePackageRoots(plugin)

	assert.Equal(t, pluginCore, got["SHOPWARE_CORE_ROOT"])
}

func TestShopwarePackageRootsFromPlatformSrc(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src", "Core"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "Core", "Kernel.php"), []byte("<?php\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src", "Administration"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src", "Storefront"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "vendor", "shopware", "dev-tools"), 0o755))

	got := shopwarePackageRoots(root)

	assert.Equal(t, filepath.Join(root, "src", "Core"), got["SHOPWARE_CORE_ROOT"])
	assert.Equal(t, filepath.Join(root, "src", "Administration"), got["SHOPWARE_ADMIN_ROOT"])
	assert.Equal(t, filepath.Join(root, "src", "Storefront"), got["SHOPWARE_STOREFRONT_ROOT"])
	assert.Equal(t, filepath.Join(root, "vendor", "shopware", "dev-tools"), got["SHOPWARE_DEV_TOOLS_ROOT"])
	assert.NotContains(t, got, "SHOPWARE_ELASTICSEARCH_ROOT")
}

func TestShopwarePackageRootsVendorWinsOverPlatformSrc(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	vendorCore := filepath.Join(root, "vendor", "shopware", "core")
	require.NoError(t, os.MkdirAll(vendorCore, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src", "Core"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "Core", "Kernel.php"), []byte("<?php\n"), 0o644))

	got := shopwarePackageRoots(root)

	assert.Equal(t, vendorCore, got["SHOPWARE_CORE_ROOT"])
}

func TestShopwarePackageRootsMissing(t *testing.T) {
	t.Parallel()

	assert.Nil(t, shopwarePackageRoots(t.TempDir()))
}

func TestShopwarePackageRootsFollowsSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(root, "checkouts", "core")
	link := filepath.Join(root, "vendor", "shopware", "core")
	require.NoError(t, os.MkdirAll(target, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(link), 0o755))
	require.NoError(t, os.Symlink(target, link))

	got := shopwarePackageRoots(root)

	assert.Equal(t, link, got["SHOPWARE_CORE_ROOT"])
}

func TestAppendShopwarePackageEnvKeepsExistingValue(t *testing.T) {
	t.Parallel()

	env := appendShopwarePackageEnv(
		[]string{"SHOPWARE_CORE_ROOT=/custom/core", "OTHER=1"},
		map[string]string{
			"SHOPWARE_CORE_ROOT":       "/vendor/shopware/core",
			"SHOPWARE_STOREFRONT_ROOT": "/vendor/shopware/storefront",
		},
	)

	assert.Equal(t, []string{
		"SHOPWARE_CORE_ROOT=/custom/core",
		"OTHER=1",
		"SHOPWARE_STOREFRONT_ROOT=/vendor/shopware/storefront",
	}, env)
}

func TestPhpStanCommandEnv(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "vendor", "shopware", "core"), 0o755))

	env := phpStanCommandEnv([]string{"KEEP=1"}, ToolConfig{RootDir: root, ToolDirectory: "/tools"})

	assert.Equal(t, []string{
		"KEEP=1",
		"PHP_DIR=/tools/php",
		"SHOPWARE_CORE_ROOT=" + filepath.Join(root, "vendor", "shopware", "core"),
	}, env)
}
