package system

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDockerRunCommandCancellation(t *testing.T) {
	for _, failCleanup := range []bool{false, true} {
		t.Run(map[bool]string{false: "stops container", true: "reports cleanup failure"}[failCleanup], func(t *testing.T) {
			dir := t.TempDir()
			ready := filepath.Join(dir, "ready")
			removed := filepath.Join(dir, "removed")
			t.Setenv("DOCKER_READY", ready)
			t.Setenv("DOCKER_REMOVED", removed)
			if failCleanup {
				t.Setenv("DOCKER_FAIL_CLEANUP", "1")
			} else {
				t.Setenv("DOCKER_FAIL_CLEANUP", "")
			}
			require.NoError(t, os.WriteFile(filepath.Join(dir, "docker"), []byte(`#!/bin/sh
case "$1" in
run)
 printf '%s' "$3" > "$DOCKER_READY"
 exec sleep 30
 ;;
rm)
 if [ "$DOCKER_FAIL_CLEANUP" = 1 ]; then echo 'daemon unavailable' >&2; exit 1; fi
 printf '%s' "$3" > "$DOCKER_REMOVED"
 ;;
esac
`), 0o755))
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			cmd, finish := DockerRunCommand(ctx, "--rm", "image", "composer", "install")
			require.NoError(t, cmd.Start())
			require.Eventually(t, func() bool { data, _ := os.ReadFile(ready); return len(data) > 0 }, 5*time.Second, 10*time.Millisecond)
			cancel()
			err := finish(cmd.Wait())
			require.Error(t, err)
			if failCleanup {
				assert.ErrorIs(t, err, ErrDockerCleanup)
				return
			}
			assert.NotErrorIs(t, err, ErrDockerCleanup)
			name, err := os.ReadFile(ready)
			require.NoError(t, err)
			assert.Contains(t, string(name), "shopware-cli-install-")
			stopped, err := os.ReadFile(removed)
			require.NoError(t, err)
			assert.Equal(t, name, stopped)
		})
	}
}

func TestDockerRunCommandExit(t *testing.T) {
	tests := []struct {
		name           string
		runExit        string
		cleanup        string
		wantErr        bool
		wantCleanup    bool
		wantCleanupErr bool
	}{
		{name: "successful install", runExit: "0"},
		{name: "failed client", runExit: "1", wantErr: true, wantCleanup: true},
		{name: "already removed container", runExit: "1", cleanup: "missing", wantErr: true, wantCleanup: true},
		{name: "cleanup failure", runExit: "1", cleanup: "failed", wantErr: true, wantCleanup: true, wantCleanupErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			removed := filepath.Join(dir, "removed")
			t.Setenv("DOCKER_REMOVED", removed)
			t.Setenv("DOCKER_RUN_EXIT", test.runExit)
			t.Setenv("DOCKER_CLEANUP", test.cleanup)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "docker"), []byte(`#!/bin/sh
case "$1" in
run) exit "$DOCKER_RUN_EXIT" ;;
rm)
 printf '%s' "$3" > "$DOCKER_REMOVED"
 case "$DOCKER_CLEANUP" in
 missing) echo 'Error response from daemon: No such container' >&2; exit 1 ;;
 failed) echo 'daemon unavailable' >&2; exit 1 ;;
 esac
 ;;
esac
`), 0o755))
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			cmd, finish := DockerRunCommand(t.Context(), "--rm", "image")
			runErr := cmd.Run()
			err := finish(runErr)
			if test.wantErr {
				require.Error(t, runErr)
				assert.ErrorIs(t, err, runErr)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, test.wantCleanupErr, errors.Is(err, ErrDockerCleanup))
			if test.wantCleanup {
				assert.FileExists(t, removed)
			} else {
				assert.NoFileExists(t, removed)
			}
		})
	}
}

func TestDockerRunCommandStartFailure(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	cmd, finish := DockerRunCommand(t.Context(), "--rm", "image")
	err := cmd.Run()
	require.Error(t, err)
	assert.ErrorIs(t, finish(err), err)
	assert.NotErrorIs(t, finish(err), ErrDockerCleanup)
}
