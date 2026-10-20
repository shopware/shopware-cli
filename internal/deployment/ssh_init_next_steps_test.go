package deployment

import (
	"bytes"
	"context"
	"io"
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

func TestInitializationNextStepsPreserveCommandContext(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("generated command targets a POSIX shell")
	}
	caller := t.TempDir()
	t.Chdir(caller)
	root := filepath.Join(caller, "shop's checkout")
	config := "./custom ' project.yml"
	environment := "prod'$(touch unexpected)"
	backend, err := New(root, config, &shop.Config{}, &shop.EnvironmentConfig{
		Type: "ssh", SSH: &shop.EnvironmentSSHConfig{Host: "web.example.invalid", Directory: "/srv/shop/live"},
	})
	require.NoError(t, err)
	var output bytes.Buffer
	require.NoError(t, backend.(InitializationGuide).WriteInitializationNextSteps(environment, &output))
	assert.Contains(t, output.String(), `"/srv/shop/shared/.env.local"`)
	assert.Contains(t, output.String(), `"/srv/shop/live/public"`)
	var command string
	for _, line := range strings.Split(output.String(), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "shopware-cli ") {
			command = strings.TrimSpace(line)
		}
	}
	require.NotEmpty(t, command)
	bin := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(bin, "shopware-cli"), "#!/bin/sh\nprintf '%s\\000' \"$@\"\n")
	require.NoError(t, os.Chmod(filepath.Join(bin, "shopware-cli"), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	// Run only the recording stub to verify the printed command's arguments.
	recorded, err := exec.CommandContext(t.Context(), "sh", "-c", command).Output()
	require.NoError(t, err)
	assert.Equal(t, []string{
		"project", "deploy", "create", "-e", environment,
		"--project-config", filepath.Join(caller, config), "--rollout", "--", root,
	}, strings.Split(strings.TrimSuffix(string(recorded), "\x00"), "\x00"))
	assert.NoFileExists(t, filepath.Join(caller, "unexpected"))
}

func TestInitializationNextStepsIncludeAllTargets(t *testing.T) {
	backend, err := New(t.TempDir(), "", &shop.Config{}, &shop.EnvironmentConfig{
		Type: "ssh", SSH: &shop.EnvironmentSSHConfig{
			Directory: "/srv/shop/live", MigrationHost: "b",
			Hosts: map[string]*shop.EnvironmentSSHHostConfig{
				"a": {Host: "first.example.invalid"},
				"b": {Host: "second.example.invalid", Directory: "/srv/other/shop"},
			},
		},
	})
	require.NoError(t, err)
	guide := backend.(InitializationGuide)
	var output bytes.Buffer
	require.NoError(t, guide.WriteInitializationNextSteps("", &output))
	assert.Contains(t, output.String(), `"first.example.invalid": "/srv/shop/shared/.env.local"`)
	assert.Contains(t, output.String(), `"second.example.invalid": "/srv/other/shared/.env.local"`)
	assert.Contains(t, output.String(), `"first.example.invalid": "/srv/shop/live/public"`)
	assert.Contains(t, output.String(), `"second.example.invalid": "/srv/other/shop/public"`)
	assert.NotContains(t, output.String(), " -e ")
	assert.NotContains(t, output.String(), "--project-config")
	reader, writer := io.Pipe()
	require.NoError(t, reader.Close())
	require.NoError(t, writer.Close())
	require.ErrorIs(t, guide.WriteInitializationNextSteps("", writer), io.ErrClosedPipe)
}

func TestSSHInitializationCancellationDoesNotWrite(t *testing.T) {
	backend := localDeploymentSSH(t)
	root, err := backend.deploymentRoot()
	require.NoError(t, err)
	before, err := os.ReadFile(filepath.Join(root, "shared/.env.local"))
	require.NoError(t, err)
	err = backend.initializeDeployment(t.Context(), func(context.Context, sshDeploymentInitState) (sshDeploymentInitConfig, bool, error) {
		return sshDeploymentInitConfig{RuntimeValues: map[string]string{"APP_SECRET": "not-written"}}, false, nil
	})
	require.ErrorIs(t, err, ErrInitializationCanceled)
	after, err := os.ReadFile(filepath.Join(root, "shared/.env.local"))
	require.NoError(t, err)
	assert.Equal(t, before, after)
}
