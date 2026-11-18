package ssh

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
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

func TestSSHRolloutWithHTTPUser(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires Linux POSIX ACLs")
	}
	for _, bin := range []string{"php", "setfacl", "getfacl"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("requires %s", bin)
		}
	}
	// Network-isolated CI may map only the current UID into its user namespace.
	probe := t.TempDir()
	if out, err := exec.CommandContext(t.Context(), "setfacl", "-m", "u:nobody:rwx", probe).CombinedOutput(); err != nil {
		t.Skipf("test filesystem must support ACLs for a second UID: %s", out)
	}
	if user, err := exec.CommandContext(t.Context(), "id", "-un").Output(); err == nil && strings.TrimSpace(string(user)) == "nobody" {
		t.Skip("requires a deployment user distinct from nobody")
	}
	root := t.TempDir()
	t.Cleanup(func() {
		require.NoError(t, exec.CommandContext(context.WithoutCancel(t.Context()), "chmod", "-R", "u+rwX", root).Run())
	})
	testhelper.WriteFile(t, filepath.Join(root, "shared/.env.local"), "APP_ENV=prod")
	require.NoError(t, os.Chmod(filepath.Join(root, "shared/.env.local"), 0o600))
	archive := deploymentTestArchive(t, map[string]string{
		"vendor/bin/shopware-deployment-helper": `<?php file_put_contents('var/cache/generated', 'cached');`,
		"public/media/seed":                     "media",
	})

	s := SSH{env: &shop.EnvironmentConfig{SSH: &shop.EnvironmentSSHConfig{HTTPUser: "nobody"}}}
	input := sshRolloutInput{
		Root: root, Release: "release", Deployment: "release", Reference: "event",
		SHA256: fmt.Sprintf("%x", sha256.Sum256(archive)), Size: int64(len(archive)),
		SharedFiles: []string{".env.local"}, SharedDirectories: []string{"public/media"},
	}
	run := func(input sshRolloutInput, stdin []byte) ([]byte, error) {
		cmd, program := localDeploymentCommand(t, "deploy", s.rolloutInput(input))
		cmd.Stdin = io.MultiReader(program, bytes.NewReader(stdin))
		return cmd.CombinedOutput()
	}
	out, err := run(input, append(archive, "CONTINUE event\nACTIVATE event\n"...))
	require.NoError(t, err, "%s", out)
	acl := func(path string) string {
		out, err := exec.CommandContext(t.Context(), "getfacl", "-cp", filepath.Join(root, path)).CombinedOutput()
		require.NoError(t, err, "%s", out)
		return string(out)
	}
	assert.Contains(t, acl("shared/.env.local"), "user:nobody:r--")
	assert.Contains(t, acl("shared/public/media"), "default:user:nobody:rwx")
	assert.Contains(t, acl("shared/public/media/seed"), "user:nobody:rw-")
	assert.Contains(t, acl("current/var/cache/generated"), "user:nobody:")
	assert.NotContains(t, acl(".shopware-cli"), "user:nobody:")
	assert.NotContains(t, acl("current/vendor/bin/shopware-deployment-helper"), "user:nobody:")

	out, err = run(input, nil)
	require.NoError(t, err, "%s", out)
	assert.Contains(t, string(out), "UNCHANGED")
	s.env.SSH.HTTPUser = ""
	_, err = run(input, nil)
	require.Error(t, err)

	candidates := sshRolloutInput{Root: root, Action: sshDeploymentCandidates}
	out, err = run(candidates, nil)
	require.NoError(t, err, "%s", out)
	assert.JSONEq(t, "[]", string(out))

	s.env.SSH.HTTPUser = "nobody"
	second := input
	second.Release, second.Deployment, second.Reference = "second", "second", "second-event"
	out, err = run(second, []byte("CONTINUE second-event\nACTIVATE second-event\n"))
	require.NoError(t, err, "%s", out)
	rollback := sshRolloutInput{Root: root, Action: sshDeploymentActivate, Release: "release", Reference: "rollback"}
	out, err = run(rollback, []byte("ACTIVATE rollback\n"))
	require.NoError(t, err, "%s", out)
	runLocalDeploymentCommand(t, "prune", map[string]any{"root": root, "keep": 0, "dry_run": false})
	assert.NoDirExists(t, filepath.Join(root, "runtime", "second"))
	assert.DirExists(t, filepath.Join(root, "runtime", "release"))

	// A host that refuses ACL changes must keep serving the previous release.
	bin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "setfacl"), []byte("#!/bin/sh\nexit 1\n"), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	input.Release, input.Reference = "third", "third-event"
	_, err = run(input, []byte("CONTINUE third-event\nACTIVATE third-event\n"))
	require.Error(t, err)
	target, err := os.Readlink(filepath.Join(root, "current"))
	require.NoError(t, err)
	assert.Equal(t, "releases/release", target)
}
