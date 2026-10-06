package deployment

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

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
	dotted := filepath.Join(root, ".shopware-cli", "deployments", "release.v2.tar.gz")
	testhelper.WriteFile(t, dotted, "archive")
	assert.Equal(t, dotted, resolveDeploymentArchive(root, "release.v2"))
}

func TestSSHRolloutCandidates(t *testing.T) {
	s := &SSH{root: t.TempDir()}
	references, err := s.RolloutCandidates(t.Context())
	require.NoError(t, err)
	assert.Empty(t, references)
	dir := filepath.Join(s.root, ".shopware-cli", "deployments")
	older := time.Date(2026, 9, 22, 11, 0, 0, 0, time.UTC)
	newest := older.Add(time.Hour)
	for _, name := range []string{"zebra.tar.gz", "alpha-two.tar.gz", "alpha-one.tar.gz", "alpha.tar.gz", "ignored.txt"} {
		testhelper.WriteFile(t, filepath.Join(dir, name), "archive")
		require.NoError(t, os.Chtimes(filepath.Join(dir, name), older, older))
	}
	require.NoError(t, os.Chtimes(filepath.Join(dir, "zebra.tar.gz"), newest, newest))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "directory.tar.gz"), 0o755))
	if runtime.GOOS != "windows" {
		require.NoError(t, os.Symlink(filepath.Join(dir, "zebra.tar.gz"), filepath.Join(dir, "link.tar.gz")))
		assert.Equal(t, "link", resolveDeploymentArchive(s.root, "link"))
	}
	references, err = s.RolloutCandidates(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []Candidate{
		{Deployment: Deployment{Reference: "zebra", Name: "zebra"}, BuiltAt: &newest},
		{Deployment: Deployment{Reference: "alpha", Name: "alpha"}, BuiltAt: &older},
		{Deployment: Deployment{Reference: "alpha-one", Name: "alpha-one"}, BuiltAt: &older},
		{Deployment: Deployment{Reference: "alpha-two", Name: "alpha-two"}, BuiltAt: &older},
	}, references)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = s.RolloutCandidates(ctx)
	require.ErrorIs(t, err, context.Canceled)
}
