//go:build deployment

package project

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/testhelper"
)

func newArchiveTestCommand(t *testing.T, args []string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	previousConfig, previousEnv := projectConfigPath, environmentName
	t.Cleanup(func() { projectConfigPath, environmentName = previousConfig, previousEnv })
	root := &cobra.Command{Use: "project", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().StringVar(&projectConfigPath, "project-config", "", "")
	root.PersistentFlags().StringVarP(&environmentName, "env", "e", "", "")
	deployment := &cobra.Command{Use: "deployment"}
	pack := &cobra.Command{Use: "package"}
	archive := &cobra.Command{
		Use:  projectDeploymentPackageArchiveCmd.Use,
		Args: projectDeploymentPackageArchiveCmd.Args,
		RunE: projectDeploymentPackageArchiveCmd.RunE,
	}
	archive.Flags().StringP("output", "o", "", "")
	archive.Flags().Bool("with-dev-dependencies", false, "")
	root.AddCommand(deployment)
	deployment.AddCommand(pack)
	pack.AddCommand(archive)
	out := new(bytes.Buffer)
	root.SetOut(out)
	root.SetErr(out)
	root.SetArgs(append([]string{"deployment", "package", "archive"}, args...))
	return root, out
}

func TestDeploymentPackageArchiveCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses shell hooks and a PHP stand-in")
	}
	t.Setenv("PROJECT_ROOT", "")
	t.Setenv("PHP_BINARY", "")
	phpDir := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(phpDir, "php"), "#!/bin/sh\n[ \"$1\" = bin/ci ] || exit 1\n[ \"$COMPOSER_MIRROR_PATH_REPOS\" = 1 ] || exit 1\nprintf '%s' \"$PROJECT_ROOT\" > php-root.txt\n")
	require.NoError(t, os.Chmod(filepath.Join(phpDir, "php"), 0o755))
	t.Setenv("PATH", phpDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	for _, mode := range []string{"explicit path", "closest project", "custom config"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			workDir := t.TempDir()
			t.Chdir(workDir)
			testhelper.WriteFile(t, filepath.Join(root, "composer.json"), `{"require":{"shopware/core":"6.7.0.0"}}`)
			testhelper.WriteFile(t, filepath.Join(root, "bin/console"), "<?php")
			testhelper.WriteFile(t, filepath.Join(root, "source.txt"), "source")
			configPath := filepath.Join(root, ".shopware-project.yaml")
			if mode == "custom config" {
				configPath = filepath.Join(workDir, "custom.yml")
			}
			testhelper.WriteFile(t, configPath, `
compatibility_date: "2026-01-01"
disable_composer_install: true
build:
  cleanup_paths: [source.txt]
  disable_checksums: true
  hooks:
    pre: ["printf built > generated.txt"]
    post: ["test ! -f source.txt"]
environments:
  production:
    type: ssh
    ssh:
      host: example.invalid
      directory: /var/www/shopware
deployment:
  staging:
    enabled: true
`)
			output := filepath.Join(workDir, "shopware.tar.gz")
			args := []string{root, "-e", "production", "-o", output}
			if mode == "closest project" {
				t.Chdir(filepath.Join(root, "bin"))
				args = []string{"-e", "production", "-o", output}
			}
			if mode == "custom config" {
				args = append(args, "--project-config", "custom.yml")
			}
			cmd, out := newArchiveTestCommand(t, args)
			require.NoError(t, cmd.ExecuteContext(t.Context()))
			assert.Equal(t, output+"\n", out.String())
			assert.FileExists(t, filepath.Join(root, "source.txt"))
			assert.NoFileExists(t, filepath.Join(root, "generated.txt"))
			assert.NoDirExists(t, filepath.Join(root, "vendor"))

			file, err := os.Open(output)
			require.NoError(t, err)
			defer func() { assert.NoError(t, file.Close()) }()
			gz, err := gzip.NewReader(file)
			require.NoError(t, err)
			defer func() { assert.NoError(t, gz.Close()) }()
			tr := tar.NewReader(gz)
			contents := map[string]string{}
			for {
				header, err := tr.Next()
				if errors.Is(err, io.EOF) {
					break
				}
				require.NoError(t, err)
				data, err := io.ReadAll(tr)
				require.NoError(t, err)
				contents[header.Name] = string(data)
			}
			assert.Equal(t, "built", contents["generated.txt"])
			assert.NotContains(t, contents, "source.txt")
			assert.Contains(t, contents[".shopware-project.yml"], "staging:")
			assert.NotContains(t, contents[".shopware-project.yml"], "environments:")
			stage := contents["php-root.txt"]
			assert.Contains(t, stage, "shopware-deployment-")
			assert.NoDirExists(t, stage)
			assert.NotEqual(t, root, stage)
		})
	}
}

func TestDeploymentPackageArchiveRejectsInvalidInput(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
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
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			cmd, out := newArchiveTestCommand(t, tc.args)
			require.ErrorContains(t, cmd.ExecuteContext(t.Context()), tc.want)
			assert.Empty(t, out.String())
			assert.NoDirExists(t, filepath.Join(root, ".shopware-cli"))
		})
	}
}
