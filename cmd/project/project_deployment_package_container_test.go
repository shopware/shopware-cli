//go:build deployment

package project

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/testhelper"
)

func newContainerTestCommand(t *testing.T, args []string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	previousConfig, previousEnv := projectConfigPath, environmentName
	t.Cleanup(func() { projectConfigPath, environmentName = previousConfig, previousEnv })
	root := &cobra.Command{Use: "project", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().StringVar(&projectConfigPath, "project-config", "", "")
	root.PersistentFlags().StringVarP(&environmentName, "env", "e", "", "")
	deployment := &cobra.Command{Use: "deployment"}
	pack := &cobra.Command{Use: "package"}
	container := &cobra.Command{
		Use:  projectDeploymentPackageContainerCmd.Use,
		Args: projectDeploymentPackageContainerCmd.Args,
		RunE: projectDeploymentPackageContainerCmd.RunE,
	}
	container.Flags().String("php-version", "", "")
	container.Flags().StringArrayP("tag", "t", nil, "")
	container.Flags().String("platform", "", "")
	container.Flags().Bool("load", false, "")
	container.Flags().Bool("push", false, "")
	container.Flags().Bool("with-dev-dependencies", false, "")
	root.AddCommand(deployment)
	deployment.AddCommand(pack)
	pack.AddCommand(container)
	out, logs := new(bytes.Buffer), new(bytes.Buffer)
	root.SetOut(out)
	root.SetErr(logs)
	root.SetArgs(append([]string{"deployment", "package", "container"}, args...))
	return root, out, logs
}

func TestDeploymentPackageContainerCommand(t *testing.T) {
	// File generation must work with neither Docker nor valid credentials.
	t.Setenv("PATH", t.TempDir())
	t.Setenv("PROJECT_ROOT", "")
	t.Setenv("COMPOSER_AUTH", "invalid auth must not be read")
	t.Setenv("SHOPWARE_PACKAGES_TOKEN", "private-token")
	for _, mode := range []string{"explicit path", "closest project", "custom config"} {
		t.Run(mode, func(t *testing.T) {
			root, caller := t.TempDir(), t.TempDir()
			t.Chdir(caller)
			testhelper.WriteFile(t, filepath.Join(root, "composer.json"), `{"require":{"shopware/core":"6.7.0.0"}}`)
			testhelper.WriteFile(t, filepath.Join(root, "bin/console"), "<?php")
			configPath := filepath.Join(root, ".shopware-project.yml")
			args := []string{root, "--with-dev-dependencies", "--load", "--push", "-t", "my-shop:release", "-t", "my-shop:latest", "--platform", "linux/amd64", "-e", "production"}
			if mode == "closest project" {
				t.Chdir(filepath.Join(root, "bin"))
				args = args[1:]
			}
			if mode == "custom config" {
				configPath = filepath.Join(caller, "custom.yml")
				args = append(args, "--project-config", "custom.yml")
			}
			config := `compatibility_date: "2026-01-01"
php_version: "8.4"
environments:
  production:
    type: ssh
    ssh:
      host: example.invalid
      directory: /var/www/shopware
`
			testhelper.WriteFile(t, configPath, config)
			cmd, out, logs := newContainerTestCommand(t, args)
			require.NoError(t, cmd.ExecuteContext(t.Context()))
			assert.Equal(t, "docker buildx build --tag 'my-shop:release' --tag 'my-shop:latest' --platform 'linux/amd64' --load --push '"+root+"'\n", out.String())
			assert.Contains(t, logs.String(), "Generated "+filepath.Join(root, "Dockerfile"))
			assert.Contains(t, logs.String(), "Generated "+filepath.Join(root, ".dockerignore"))
			data, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
			require.NoError(t, err)
			assert.Contains(t, string(data), "ARG PHP_VERSION=8.4")
			assert.Contains(t, string(data), "--with-dev-dependencies")
			assert.NotContains(t, string(data), "project_config")
			assert.NotContains(t, string(data), "private-token")
			assert.NotContains(t, out.String()+logs.String(), "private-token")
			assert.FileExists(t, filepath.Join(root, ".dockerignore"))
			assert.NoFileExists(t, filepath.Join(caller, "Dockerfile"))
			data, err = os.ReadFile(configPath)
			require.NoError(t, err)
			assert.Equal(t, config, string(data))
		})
	}
}

func TestDeploymentPackageContainerOutputOptions(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	for _, tc := range []struct {
		name  string
		flags []string
		load  bool
		push  bool
	}{
		{"default", nil, false, false},
		{"load", []string{"--load"}, true, false},
		{"push", []string{"--push"}, false, true},
		{"both", []string{"--load", "--push"}, true, true},
		{"disabled", []string{"--load=false", "--push=false"}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			cmd, out, _ := newContainerTestCommand(t, append([]string{root}, tc.flags...))
			require.NoError(t, cmd.ExecuteContext(t.Context()))
			assert.Equal(t, tc.load, strings.Contains(out.String(), " --load"))
			assert.Equal(t, tc.push, strings.Contains(out.String(), " --push"))
			assert.FileExists(t, filepath.Join(root, "Dockerfile"))
		})
	}
}

func TestDeploymentPackageContainerDetectsPHP(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("PROJECT_ROOT", "")
	for _, tc := range []struct {
		name, config, localConfig, lock, want string
	}{
		{
			name:   "Docker config",
			config: "docker:\n  php:\n    version: '8.4'\n",
			want:   "8.4",
		},
		{
			name:        "local Docker override",
			config:      "docker:\n  php:\n    version: '8.3'\n",
			localConfig: "docker:\n  php:\n    version: '8.4'\n",
			want:        "8.4",
		},
		{
			name: "project lockfile",
			lock: `{"packages":[{"name":"shopware/core","require":{"php":"~8.4.0"}}]}`,
			want: "8.4",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			testhelper.WriteFile(t, filepath.Join(root, "composer.json"), `{"require":{"shopware/core":"6.7.0.0"}}`)
			testhelper.WriteFile(t, filepath.Join(root, "bin/console"), "<?php")
			testhelper.WriteFile(t, filepath.Join(root, ".shopware-project.yaml"), "compatibility_date: '2026-01-01'\n"+tc.config)
			if tc.localConfig != "" {
				testhelper.WriteFile(t, filepath.Join(root, ".shopware-project.local.yaml"), tc.localConfig)
			}
			if tc.lock != "" {
				testhelper.WriteFile(t, filepath.Join(root, "composer.lock"), tc.lock)
			}
			t.Chdir(filepath.Join(root, "bin"))
			// A file in the caller's directory must not influence project detection.
			testhelper.WriteFile(t, filepath.Join(root, "bin/composer.lock"), "invalid")
			cmd, out, _ := newContainerTestCommand(t, nil)
			require.NoError(t, cmd.ExecuteContext(t.Context()))
			assert.Contains(t, out.String(), "docker buildx build")
			data, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
			require.NoError(t, err)
			assert.Contains(t, string(data), "ARG PHP_VERSION="+tc.want+"\n")
		})
	}
}

func TestDeploymentPackageContainerRejectsInvalidInput(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("PATH", t.TempDir())
	t.Setenv("PROJECT_ROOT", "")
	testhelper.WriteFile(t, filepath.Join(root, "composer.json"), `{"require":{"shopware/core":"6.7.0.0"}}`)
	testhelper.WriteFile(t, filepath.Join(root, "bin/console"), "<?php")
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{".", "."}, "accepts at most 1 arg"},
		{[]string{"missing"}, "read project directory"},
		{[]string{"composer.json"}, "not a directory"},
		{[]string{"-e", "missing"}, `environment "missing" not found`},
		{[]string{"--generate"}, "unknown flag"},
		{[]string{"--tag", " "}, "image tag must not be empty"},
	} {
		cmd, out, _ := newContainerTestCommand(t, tc.args)
		require.ErrorContains(t, cmd.ExecuteContext(t.Context()), tc.want)
		assert.Empty(t, out.String())
		assert.NoFileExists(t, filepath.Join(root, "Dockerfile"))
	}
	testhelper.WriteFile(t, filepath.Join(root, ".dockerignore"), "original")
	cmd, out, _ := newContainerTestCommand(t, nil)
	require.ErrorContains(t, cmd.ExecuteContext(t.Context()), "refusing to overwrite")
	assert.Empty(t, out.String())
	assert.NoFileExists(t, filepath.Join(root, "Dockerfile"))
}
