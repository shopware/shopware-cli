package system

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsDockerUsingLibkrun(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Run("file not found returns true", func(t *testing.T) {
			tmpDir := t.TempDir()
			t.Setenv("HOME", tmpDir)
			assert.True(t, IsDockerUsingLibkrun())
		})

		t.Run("UseLibkrun true returns true", func(t *testing.T) {
			tmpDir := t.TempDir()
			t.Setenv("HOME", tmpDir)

			settingsFile := filepath.Join(tmpDir, "Library/Group Containers/group.com.docker/settings-store.json")
			assert.NoError(t, os.MkdirAll(filepath.Dir(settingsFile), 0755))
			assert.NoError(t, os.WriteFile(settingsFile, []byte(`{"UseLibkrun":true}`), 0644))

			assert.True(t, IsDockerUsingLibkrun())
		})

		t.Run("UseLibkrun false returns false", func(t *testing.T) {
			tmpDir := t.TempDir()
			t.Setenv("HOME", tmpDir)

			settingsFile := filepath.Join(tmpDir, "Library/Group Containers/group.com.docker/settings-store.json")
			assert.NoError(t, os.MkdirAll(filepath.Dir(settingsFile), 0755))
			assert.NoError(t, os.WriteFile(settingsFile, []byte(`{"UseLibkrun":false}`), 0644))

			assert.False(t, IsDockerUsingLibkrun())
		})

		t.Run("invalid json returns true", func(t *testing.T) {
			tmpDir := t.TempDir()
			t.Setenv("HOME", tmpDir)

			settingsFile := filepath.Join(tmpDir, "Library/Group Containers/group.com.docker/settings-store.json")
			assert.NoError(t, os.MkdirAll(filepath.Dir(settingsFile), 0755))
			assert.NoError(t, os.WriteFile(settingsFile, []byte(`invalid json`), 0644))

			assert.True(t, IsDockerUsingLibkrun())
		})
	} else {
		t.Run("non-darwin returns false", func(t *testing.T) {
			t.Setenv("HOME", "/tmp/test-home")
			assert.False(t, IsDockerUsingLibkrun())
		})
	}
}

func TestCheckIncompatibilities(t *testing.T) {
	t.Run("no incompatibilities on non-darwin", func(t *testing.T) {
		t.Setenv("HOME", "/tmp/test-home")
		incompatibilities := CheckIncompatibilities(false, "/tmp/project")
		assert.Empty(t, incompatibilities)
	})
}
