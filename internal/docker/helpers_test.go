package docker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/shyim/go-composer"
	"github.com/stretchr/testify/require"
)

// envFor builds an environment for in-package tests: the lock decides the
// optional services, base carries every other input.
func envFor(lock *composer.Lock, base Environment) *Environment {
	base.features = featuresFromLock(lock)
	return &base
}

// writeLock writes a composer.lock into dir listing shopware/core plus the
// given packages.
func writeLock(t *testing.T, dir string, packages ...string) {
	t.Helper()

	type pkg struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	lock := struct {
		Packages    []pkg `json:"packages"`
		PackagesDev []pkg `json:"packages-dev"`
	}{Packages: []pkg{{Name: "shopware/core", Version: "6.6.0.0"}}, PackagesDev: []pkg{}}
	for _, name := range packages {
		lock.Packages = append(lock.Packages, pkg{Name: name, Version: "1.0.0"})
	}

	content, err := json.Marshal(lock)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "composer.lock"), content, 0o644))
}

// writeCompose resolves the project's environment with default options and
// writes its compose file.
func writeCompose(t *testing.T, dir string) error {
	t.Helper()

	env, err := NewEnvironment(dir, Options{})
	if err != nil {
		return err
	}

	return env.WriteCompose()
}
