package deployment

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestResolveDeploymentArchive(t *testing.T) {
	root := t.TempDir()
	archive := filepath.Join(root, ".shopware-cli", "deployments", "focused-turing.tar.gz")
	testhelper.WriteFile(t, archive, "archive")

	assert.Equal(t, archive, resolveDeploymentArchive(root, "focused-turing"))
	assert.Equal(t, archive, resolveDeploymentArchive(root, "focused-turing.tar.gz"))
	assert.Equal(t, "missing-name", resolveDeploymentArchive(root, "missing-name"))
	assert.Equal(t, "./focused-turing", resolveDeploymentArchive(root, "./focused-turing"))
}

func TestSSHCompleteDeploymentReferences(t *testing.T) {
	s := &SSH{root: t.TempDir()}
	references, err := s.CompleteDeploymentReferences(t.Context(), "")
	require.NoError(t, err)
	assert.Empty(t, references)
	dir := filepath.Join(s.root, ".shopware-cli", "deployments")
	for _, name := range []string{"zebra.tar.gz", "alpha-two.tar.gz", "alpha-one.tar.gz", "ignored.txt"} {
		testhelper.WriteFile(t, filepath.Join(dir, name), "archive")
	}
	require.NoError(t, os.Mkdir(filepath.Join(dir, "directory.tar.gz"), 0o755))
	if runtime.GOOS != "windows" {
		require.NoError(t, os.Symlink(filepath.Join(dir, "zebra.tar.gz"), filepath.Join(dir, "link.tar.gz")))
		assert.Equal(t, "link", resolveDeploymentArchive(s.root, "link"))
	}
	references, err = s.CompleteDeploymentReferences(t.Context(), "")
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha-one", "alpha-two", "zebra"}, references)
	references, err = s.CompleteDeploymentReferences(t.Context(), "alpha")
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha-one", "alpha-two"}, references)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = s.CompleteDeploymentReferences(ctx, "")
	require.ErrorIs(t, err, context.Canceled)
}
