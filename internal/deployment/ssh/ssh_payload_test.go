package ssh

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/phar"
)

func TestDeploymentPayloadCommands(t *testing.T) {
	root := t.TempDir()
	secret := "secret with 'quotes' and $dollars"
	runLocalDeploymentInit(t, sshDeploymentInitInput{Action: sshDeploymentInitApply, Root: root, RuntimeValues: map[string]string{"APP_SECRET": secret}})
	var state sshDeploymentInitState
	output := runLocalDeploymentInit(t, sshDeploymentInitInput{Action: sshDeploymentInitInspect, Root: root})
	require.NoError(t, json.Unmarshal(output, &state))
	assert.True(t, state.Initialized)
	assert.True(t, state.HasValidRuntimeConfig())
	assert.Equal(t, []string{"APP_SECRET"}, state.RuntimeKeys)
	assert.NotContains(t, string(output), secret)
	info, err := os.Stat(filepath.Join(root, "shared", ".env.local"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
	for _, command := range []string{"list", "logs", "deploy"} {
		output := runLocalDeploymentCommand(t, command, map[string]any{"root": root, "list": true, "action": "candidates"})
		assert.JSONEq(t, "[]", string(output))
	}
	output = runLocalDeploymentCommand(t, "prune", map[string]any{"root": root, "keep": 3, "dry_run": true})
	assert.JSONEq(t, `{"deployments":[],"artifacts":[],"artifact_skip_reasons":[]}`, string(output))
}

func TestDeploymentPathJoin(t *testing.T) {
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("PHP required")
	}
	helper, err := deploymentPHP.ReadFile("php/lib/filesystem.php")
	require.NoError(t, err)
	program, err := phar.Build(fstest.MapFS{
		"filesystem.php": {Data: helper},
		"main.php": {Data: []byte(`<?php
require __DIR__ . '/filesystem.php';
$cases = json_decode($argv[1], true, 512, JSON_THROW_ON_ERROR);
echo json_encode(array_map(static fn (array $parts): string => path_join(...$parts), $cases), JSON_THROW_ON_ERROR);
`)},
	}, "main.php")
	require.NoError(t, err)
	cases := []struct {
		parts []string
		want  string
	}{
		{[]string{"/srv/shop", "releases", "release"}, "/srv/shop/releases/release"},
		{[]string{"/srv/shop///", "/releases/", "", "/release/"}, "/srv/shop/releases/release"},
		{[]string{"/", "releases"}, "/releases"},
		{[]string{"/", "", "/"}, "/"},
		{[]string{"", ""}, ""},
		{[]string{"", "releases", "release"}, "releases/release"},
		{[]string{"", "/srv/shop"}, "/srv/shop"},
		{[]string{"0"}, "0"},
		{[]string{"/srv/shop", "../shared"}, "/srv/shop/../shared"},
		{[]string{"phar:///tmp/payload.phar", "lib", "filesystem.php"}, "phar:///tmp/payload.phar/lib/filesystem.php"},
	}
	parts := make([][]string, len(cases))
	for i, tc := range cases {
		parts[i] = tc.parts
	}
	input, err := json.Marshal(parts)
	require.NoError(t, err)
	cmd := exec.CommandContext(t.Context(), php, program.PHPArgs(string(input))...)
	cmd.Stdin = program.Stdin()
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", output)
	var paths []string
	require.NoError(t, json.Unmarshal(output, &paths))
	require.Len(t, paths, len(cases))
	for i, tc := range cases {
		assert.Equal(t, tc.want, paths[i], "parts: %q", tc.parts)
	}
}

func TestDeploymentInitMerge(t *testing.T) {
	for _, tc := range []struct {
		name, content, want string
		wantError           bool
	}{
		//nolint:dupword // Both duplicate assignments must contain the updated value.
		{"duplicates", "TOKEN=old\nexport TOKEN=stale\n", "TOKEN=\"new\"\nTOKEN=\"new\"\n", false},
		{"double quoted multiline", "TOKEN=\"first\nsecond\"\n", "", true},
		{"single quoted multiline", "TOKEN='first\nsecond'\n", "", true},
		{"assignment inside multiline", "OTHER=\"first\nTOKEN=old\nlast\"\n", "", true},
		{"unterminated", "TOKEN=\"first\n", "", true},
		{"comments and escapes", "# user's config\nOTHER=\"escaped \\\" quote\" # user's note\n\nTOKEN=old\n", "# user's config\nOTHER=\"escaped \\\" quote\" # user's note\n\nTOKEN=\"new\"\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			runtime := filepath.Join(root, "shared", ".env.local")
			require.NoError(t, os.MkdirAll(filepath.Dir(runtime), 0700))
			require.NoError(t, os.WriteFile(runtime, []byte(tc.content), 0600))
			input, err := json.Marshal(sshDeploymentInitInput{Action: sshDeploymentInitApply, Root: root, RuntimeValues: map[string]string{"TOKEN": "new"}})
			require.NoError(t, err)
			cmd, archive := localDeploymentCommand(t, "init", nil)
			cmd.Stdin = io.MultiReader(archive, bytes.NewReader(input))
			output, err := cmd.CombinedOutput()
			if tc.wantError {
				require.Error(t, err)
				_, markerErr := os.Stat(filepath.Join(root, ".shopware-cli", "initialized"))
				assert.True(t, os.IsNotExist(markerErr))
			} else {
				require.NoError(t, err, "%s", output)
			}
			content, err := os.ReadFile(runtime)
			require.NoError(t, err)
			if tc.wantError {
				assert.Equal(t, tc.content, string(content))
			} else {
				assert.Equal(t, tc.want, string(content))
			}
		})
	}
}

func TestDeploymentInspectRejectsFIFO(t *testing.T) {
	mkfifo, err := exec.LookPath("mkfifo")
	if err != nil {
		t.Skip("mkfifo required")
	}
	root := t.TempDir()
	runtime := filepath.Join(root, "shared", ".env.local")
	require.NoError(t, os.MkdirAll(filepath.Dir(runtime), 0700))
	require.NoError(t, exec.CommandContext(t.Context(), mkfifo, runtime).Run())
	input, err := json.Marshal(sshDeploymentInitInput{Action: sshDeploymentInitInspect, Root: root})
	require.NoError(t, err)
	cmd, archive := localDeploymentCommand(t, "init", nil)
	// A regression must fail promptly rather than hang waiting for a FIFO writer.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	cmd = exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
	cmd.Stdin = io.MultiReader(archive, bytes.NewReader(input))
	_, err = cmd.CombinedOutput()
	require.Error(t, err)
	assert.Equal(t, 1, cmd.ProcessState.ExitCode())
}
