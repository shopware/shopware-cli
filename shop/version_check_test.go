package shop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectNothingFound(t *testing.T) {
	_, err := IsShopwareVersion(t.TempDir(), "6.4")

	assert.ErrorIs(t, err, ErrNoComposerFileFound)
}

func TestDetectPlatformTrunk(t *testing.T) {
	tmpDir := t.TempDir()

	composerJson := filepath.Join(tmpDir, "composer.json")

	jsonStruct := composerJsonStruct{
		Name: "shopware/platform",
	}

	bytes, _ := json.Marshal(jsonStruct)

	_ = os.WriteFile(composerJson, bytes, os.ModePerm)

	val, err := IsShopwareVersion(tmpDir, ">=6.3")

	assert.NoError(t, err)
	assert.True(t, val)
}

func TestDetectComposerJsonNotPlatform(t *testing.T) {
	tmpDir := t.TempDir()

	composerJson := filepath.Join(tmpDir, "composer.json")

	jsonStruct := composerJsonStruct{
		Name: "my-project",
	}

	bytes, _ := json.Marshal(jsonStruct)

	_ = os.WriteFile(composerJson, bytes, os.ModePerm)

	val, err := IsShopwareVersion(tmpDir, ">=6.3")

	assert.ErrorIs(t, err, ErrShopwareDependencyNotFound)
	assert.False(t, val)
}

func TestComposerLockMatching(t *testing.T) {
	tmpDir := t.TempDir()

	composerLock := filepath.Join(tmpDir, "composer.lock")

	jsonStruct := composerLockStruct{
		Packages: []struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}{
			{
				Name:    "shopware/core",
				Version: "6.4.0",
			},
		},
	}

	bytes, _ := json.Marshal(jsonStruct)

	_ = os.WriteFile(composerLock, bytes, os.ModePerm)

	val, err := IsShopwareVersion(tmpDir, ">=6.3")

	assert.NoError(t, err)
	assert.True(t, val)
}

func TestComposerLockNotMatching(t *testing.T) {
	tmpDir := t.TempDir()

	composerLock := filepath.Join(tmpDir, "composer.lock")

	jsonStruct := composerLockStruct{
		Packages: []struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}{
			{
				Name:    "shopware/core",
				Version: "6.4.0",
			},
		},
	}

	bytes, _ := json.Marshal(jsonStruct)

	_ = os.WriteFile(composerLock, bytes, os.ModePerm)

	val, err := IsShopwareVersion(tmpDir, "<=6.3")

	assert.NoError(t, err)
	assert.False(t, val)
}

func TestComposerLockNoDependency(t *testing.T) {
	tmpDir := t.TempDir()

	composerLock := filepath.Join(tmpDir, "composer.lock")

	jsonStruct := composerLockStruct{
		Packages: []struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}{},
	}

	bytes, _ := json.Marshal(jsonStruct)

	_ = os.WriteFile(composerLock, bytes, os.ModePerm)

	val, err := IsShopwareVersion(tmpDir, "<=6.3")

	assert.ErrorIs(t, err, ErrNoComposerFileFound)
	assert.False(t, val)
}
