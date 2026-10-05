package projectbuild

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestGenerateContainerFiles(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("COMPOSER_AUTH", "invalid auth must not be read")
	t.Setenv("SHOPWARE_PACKAGES_TOKEN", "private-token")
	for _, tc := range []struct{ configPHP, overridePHP, wantPHP string }{
		{"", "", "8.3"},
		{"8.4", "", "8.4"},
		{"8.4", "8.5", "8.5"},
	} {
		root := t.TempDir()
		testhelper.WriteFile(t, filepath.Join(root, "auth.json"), "invalid auth must not be read")
		cfg := &shop.Config{
			PHPVersion:             tc.configPHP,
			DisableComposerInstall: true,
			Build:                  &shop.ConfigBuild{DisableAssetCopy: true},
			Environments: map[string]*shop.EnvironmentConfig{
				"production": {AdminApi: &shop.ConfigAdminApi{Password: "private-password"}},
			},
		}
		paths, err := GenerateContainerFiles(t.Context(), root, cfg, ContainerOptions{
			PHPVersion: tc.overridePHP, WithDevDependencies: true,
			Load: true, Push: true, Tags: []string{"my-shop:test"}, Platform: "linux/amd64",
		})
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{filepath.Join(root, "Dockerfile"), filepath.Join(root, ".dockerignore")}, paths)
		data, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
		require.NoError(t, err)
		expected, err := shop.ProductionDockerfile(shop.ProductionDockerfileOptions{
			PHPVersion: tc.wantPHP, WithDevDependencies: true,
		})
		require.NoError(t, err)
		assert.Equal(t, string(expected), string(data))
		for _, value := range []string{root, "private-token", "private-password", "project_config", "SHOPWARE_CLI_CONFIG_HASH", "SHOPWARE_CLI_CONTAINER"} {
			assert.NotContains(t, string(data), value)
		}
		ignore, err := os.ReadFile(filepath.Join(root, ".dockerignore"))
		require.NoError(t, err)
		assert.Contains(t, string(ignore), shop.ProductionDockerignore())
		assert.Contains(t, string(ignore), "!vendor/**")
		assert.Contains(t, string(ignore), "!public/bundles/**")
		assert.NotContains(t, string(ignore), "Runtime deployment configuration generated")
		assert.NoFileExists(t, filepath.Join(root, ".shopware-project.yml"))
		assert.Equal(t, tc.configPHP, cfg.PHPVersion)
		info, err := os.Stat(paths[0])
		require.NoError(t, err)
		assert.Zero(t, info.Mode()&0o111, "generated files are not executable")
	}
}

func TestGenerateContainerFilesNeverOverwrites(t *testing.T) {
	for _, name := range []string{"Dockerfile", ".dockerignore"} {
		for _, kind := range []string{"file", "directory", "symlink"} {
			t.Run(name+"/"+kind, func(t *testing.T) {
				root := t.TempDir()
				path := filepath.Join(root, name)
				switch kind {
				case "file":
					testhelper.WriteFile(t, path, "original")
				case "directory":
					require.NoError(t, os.Mkdir(path, 0o755))
				case "symlink":
					if runtime.GOOS == "windows" {
						t.Skip("symlinks require privileges on Windows")
					}
					require.NoError(t, os.Symlink("missing", path))
				}
				paths, err := GenerateContainerFiles(t.Context(), root, &shop.Config{}, ContainerOptions{})
				require.ErrorContains(t, err, "refusing to overwrite")
				assert.Empty(t, paths)
				entries, err := os.ReadDir(root)
				require.NoError(t, err)
				require.Len(t, entries, 1, "neither file should be generated on a conflict")
				assert.Equal(t, name, entries[0].Name())
				if kind == "file" {
					data, err := os.ReadFile(path)
					require.NoError(t, err)
					assert.Equal(t, "original", string(data))
				}
			})
		}
	}
}

func TestGenerateContainerFilesInvalidOptionsAndCancellation(t *testing.T) {
	for _, opts := range []ContainerOptions{
		{PHPVersion: "invalid"}, {Tags: []string{" "}},
	} {
		root := t.TempDir()
		paths, err := GenerateContainerFiles(t.Context(), root, &shop.Config{}, opts)
		require.Error(t, err)
		assert.Empty(t, paths)
		assert.NoFileExists(t, filepath.Join(root, "Dockerfile"))
		assert.NoFileExists(t, filepath.Join(root, ".dockerignore"))
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	root := t.TempDir()
	paths, err := GenerateContainerFiles(ctx, root, &shop.Config{}, ContainerOptions{})
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, paths)
	assert.NoFileExists(t, filepath.Join(root, "Dockerfile"))
	assert.NoFileExists(t, filepath.Join(root, ".dockerignore"))
}

func TestContainerBuildCommand(t *testing.T) {
	for _, tc := range []struct {
		name string
		load bool
		push bool
	}{
		{"default", false, false}, {"load", true, false},
		{"push", false, true}, {"both", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := ContainerBuildCommand("/project", ContainerOptions{
				Load: tc.load, Push: tc.push, Tags: []string{"my-shop:release", "my-shop:latest"}, Platform: "linux/amd64",
			})
			assert.Equal(t, tc.load, strings.Contains(command, " --load"))
			assert.Equal(t, tc.push, strings.Contains(command, " --push"))
			assert.Contains(t, command, "--tag 'my-shop:release' --tag 'my-shop:latest'")
			assert.Contains(t, command, "--platform 'linux/amd64'")
			assert.True(t, strings.HasSuffix(command, "'/project'"))
			assert.NotContains(t, command, "--iidfile")
		})
	}
	assert.Equal(t, "docker buildx build '/project'", ContainerBuildCommand("/project", ContainerOptions{}))
}

func TestContainerBuildCommandShellQuoting(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("printed command targets POSIX shells")
	}
	dir := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(dir, "docker"), "#!/bin/sh\nprintf '%s\\n' \"$@\"\n")
	require.NoError(t, os.Chmod(filepath.Join(dir, "docker"), 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	root := "/a shop/it's $(printf injected); $HOME"
	tag := "my-shop:$(printf injected)"
	command := ContainerBuildCommand(root, ContainerOptions{Tags: []string{tag}})
	data, err := exec.CommandContext(t.Context(), "sh", "-c", command).CombinedOutput()
	require.NoError(t, err)
	assert.Equal(t, "buildx\nbuild\n--tag\n"+tag+"\n"+root+"\n", string(data))
}
