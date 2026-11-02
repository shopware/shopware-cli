package ssh

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

// deploymentPHP finds PHP for remote-script tests, skipping when POSIX tools are unavailable.
func deploymentPHP(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("SSH release layout requires a POSIX host")
	}
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("PHP CLI is required to execute the remote deployment script")
	}
	if _, err := exec.LookPath("tar"); err != nil {
		t.Skip("tar is required")
	}
	return php
}

// localDeploymentSSH creates a local SSH fixture using the default active release link.
func localDeploymentSSH(t *testing.T) *SSH {
	t.Helper()
	return localDeploymentSSHWithLink(t, "current")
}

// localDeploymentSSHWithLink executes remote commands locally against an isolated deployment layout.
func localDeploymentSSHWithLink(t *testing.T, name string) *SSH {
	t.Helper()
	php := deploymentPHP(t)
	t.Setenv("DATABASE_URL", "")
	bin := t.TempDir()
	// Execute the SSH remote command locally. No network or sshd is involved.
	testhelper.WriteFile(t, filepath.Join(bin, "ssh"), "#!/bin/sh\nif [ -n \"$SSH_ARGS_LOG\" ]; then printf '%s\\n' \"$@\" > \"$SSH_ARGS_LOG\"; fi\nfor arg do command=\"$arg\"; done\nexec /bin/sh -c \"$command\"\n")
	require.NoError(t, os.Chmod(filepath.Join(bin, "ssh"), 0o755))
	// Never classify the developer/CI machine as a hosting provider during tests.
	testhelper.WriteFile(t, filepath.Join(bin, "hostname"), "#!/bin/sh\nprintf 'fixture.example.invalid\\n'\n")
	require.NoError(t, os.Chmod(filepath.Join(bin, "hostname"), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	// Spaces and apostrophes must survive SSH's remote shell.
	root = filepath.Join(root, "shop's releases")
	testhelper.WriteFile(t, filepath.Join(root, "shared/.env.local"), "DATABASE_URL=provided-on-server\n")
	testhelper.WriteFile(t, filepath.Join(root, "shared/public/media/upload"), "persistent")
	backend, err := New(t.TempDir(), "", &shop.Config{}, &shop.EnvironmentConfig{
		Type: "ssh",
		SSH:  &shop.EnvironmentSSHConfig{Host: "example.invalid", Directory: filepath.Join(root, name), PHPBinary: php},
	})
	require.NoError(t, err)
	return backend.(*SSH)
}

// Each host is inspected once before prompting and once during verification.
// The coordinator waits between phases, so each store is used sequentially.
type memoryInitializationStore struct {
	states      [2]sshDeploymentInitState
	inspectErrs [2]error
	inspections int
	writes      []sshDeploymentInitConfig
	applyErr    error
}

// Inspect returns the next scripted inspection result.
func (s *memoryInitializationStore) Inspect(context.Context) (sshDeploymentInitState, error) {
	i := s.inspections
	s.inspections++
	return s.states[i], s.inspectErrs[i]
}

// Apply records a configuration write and returns the scripted error.
func (s *memoryInitializationStore) Apply(_ context.Context, config sshDeploymentInitConfig) error {
	s.writes = append(s.writes, config)
	return s.applyErr
}
