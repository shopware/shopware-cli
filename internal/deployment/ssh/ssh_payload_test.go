package ssh

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"testing/fstest"

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
