package projectbuild

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestRandomDeploymentName(t *testing.T) {
	for range 100 {
		name, err := randomDeploymentName()
		require.NoError(t, err)
		parts := strings.Split(name, "-")
		require.Len(t, parts, 2)
		assert.True(t, slices.Contains(deploymentAdjectives, parts[0]))
		assert.True(t, slices.Contains(deploymentPioneers, parts[1]))
	}
}

func TestAvailableDeploymentArchivePathSkipsExistingNames(t *testing.T) {
	root := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(root, ".shopware-cli/deployments/focused-turing.tar.gz"), "existing")
	names := []string{"focused-turing", "clever-hopper"}

	path, err := availableDeploymentArchivePath(root, func() (string, error) {
		name := names[0]
		names = names[1:]
		return name, nil
	})

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, ".shopware-cli/deployments/clever-hopper.tar.gz"), path)
}

func TestAvailableDeploymentArchivePathPropagatesGeneratorError(t *testing.T) {
	expected := errors.New("random source failed")
	_, err := availableDeploymentArchivePath(t.TempDir(), func() (string, error) {
		return "", expected
	})
	require.ErrorIs(t, err, expected)
}
