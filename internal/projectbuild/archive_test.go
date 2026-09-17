package projectbuild

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

func readArchive(t *testing.T, filename string) map[string]string {
	t.Helper()
	file, err := os.Open(filename)
	require.NoError(t, err)
	defer func() { assert.NoError(t, file.Close()) }()
	gz, err := gzip.NewReader(file)
	require.NoError(t, err)
	defer func() { assert.NoError(t, gz.Close()) }()
	tr := tar.NewReader(gz)
	contents := make(map[string]string)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		data, err := io.ReadAll(tr)
		require.NoError(t, err)
		contents[header.Name] = string(data)
	}
	_, err = io.Copy(io.Discard, gz)
	require.NoError(t, err)
	return contents
}

func TestPackageArchiveUsesTemporaryCopy(t *testing.T) {
	root := t.TempDir()
	sourceFiles := map[string]string{
		"composer.json":                 "{}",
		"source.txt":                    "original",
		"auth.json":                     "composer credential",
		".env":                          "DATABASE_URL=private",
		".env.local.php":                "compiled credential",
		".env.dist":                     "example",
		".shopware-project.yml":         "admin_api: private",
		".shopware-project.yaml":        "admin_api: private",
		".shopware-project.local.yml":   "admin_api: private",
		".git/config":                   "git config",
		".shopware-cli/previous.tar.gz": "previous artifact",
		"var/cache/old":                 "old cache",
		"public/media/upload":           "media",
		"public/thumbnail/upload":       "thumbnail",
		"public/sitemap/sitemap.xml":    "sitemap",
		"public/bundles/app.js":         "assets",
	}
	for name, content := range sourceFiles {
		testhelper.WriteFile(t, filepath.Join(root, name), content)
	}
	cfg := &shop.Config{
		CompatibilityDate: "2026-01-01",
		ConfigDeployment:  &shop.ConfigDeployment{},
		Environments: map[string]*shop.EnvironmentConfig{
			"production": {AdminApi: &shop.ConfigAdminApi{Password: "not-for-the-artifact"}},
		},
	}
	var stage string
	reference, err := packageArchive(t.Context(), root, cfg, ArchiveOptions{}, func(_ context.Context, dir string) error {
		stage = dir
		assert.NotEqual(t, root, dir)
		assert.FileExists(t, filepath.Join(dir, "auth.json"))
		assert.NoFileExists(t, filepath.Join(dir, ".env"))
		assert.NoDirExists(t, filepath.Join(dir, ".git"))
		assert.NoDirExists(t, filepath.Join(dir, ".shopware-cli"))
		assert.NoDirExists(t, filepath.Join(dir, "var"))
		require.NoError(t, os.Remove(filepath.Join(dir, "source.txt")))
		testhelper.WriteFile(t, filepath.Join(dir, "built.txt"), "built")
		testhelper.WriteFile(t, filepath.Join(dir, "var/cache/new"), dir)
		testhelper.WriteFile(t, filepath.Join(dir, ".env.local"), "build credential")
		return nil
	})
	require.NoError(t, err)
	assert.NoDirExists(t, filepath.Dir(stage))
	realRoot, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(realRoot, ".shopware-cli", "deployments"), filepath.Dir(reference))
	contents := readArchive(t, reference)
	assert.Equal(t, "built", contents["built.txt"])
	assert.Equal(t, "assets", contents["public/bundles/app.js"])
	assert.Equal(t, "example", contents[".env.dist"])
	assert.Contains(t, contents[".shopware-project.yml"], "deployment:")
	assert.NotContains(t, contents[".shopware-project.yml"], "admin_api")
	assert.NotContains(t, contents[".shopware-project.yml"], "environments")
	for _, name := range []string{"source.txt", "auth.json", ".env", ".env.local", ".env.local.php", ".shopware-project.yaml", ".shopware-project.local.yml", ".git", ".shopware-cli", "var", "public/media", "public/thumbnail", "public/sitemap"} {
		assert.NotContains(t, contents, name)
	}
	for name, content := range sourceFiles {
		data, err := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, err)
		assert.Equal(t, content, string(data), name)
	}
}

func TestPackageArchiveFailureCleansUp(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(t.TempDir(), "archive.tar.gz")
	buildErr := errors.New("build failed")
	var stage string
	reference, err := packageArchive(t.Context(), root, &shop.Config{}, ArchiveOptions{OutputPath: output}, func(_ context.Context, dir string) error {
		stage = dir
		return buildErr
	})
	require.ErrorIs(t, err, buildErr)
	assert.Empty(t, reference)
	assert.NoDirExists(t, filepath.Dir(stage))
	assert.NoFileExists(t, output)
}

func TestPackageArchiveNeverOverwrites(t *testing.T) {
	for _, duringBuild := range []bool{false, true} {
		t.Run(map[bool]string{false: "preexisting", true: "created during build"}[duringBuild], func(t *testing.T) {
			root := t.TempDir()
			output := filepath.Join(t.TempDir(), "archive.tar.gz")
			if !duringBuild {
				testhelper.WriteFile(t, output, "existing")
			}
			called := false
			reference, err := packageArchive(t.Context(), root, &shop.Config{}, ArchiveOptions{OutputPath: output}, func(context.Context, string) error {
				called = true
				testhelper.WriteFile(t, output, "existing")
				return nil
			})
			require.Error(t, err)
			assert.Empty(t, reference)
			assert.Equal(t, duringBuild, called)
			data, err := os.ReadFile(output)
			require.NoError(t, err)
			assert.Equal(t, "existing", string(data))
			files, err := os.ReadDir(filepath.Dir(output))
			require.NoError(t, err)
			assert.Len(t, files, 1, "no partial output remains")
		})
	}
}

func TestPackageArchiveRejectsEscapingSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require privileges on Windows")
	}
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	require.NoError(t, os.MkdirAll(root, 0o755))
	testhelper.WriteFile(t, filepath.Join(parent, "outside"), "private")
	require.NoError(t, os.Symlink("../outside", filepath.Join(root, "link")))
	called := false
	_, err := packageArchive(t.Context(), root, &shop.Config{}, ArchiveOptions{}, func(context.Context, string) error {
		called = true
		return nil
	})
	require.ErrorContains(t, err, "validate archive source")
	assert.False(t, called)
}

func TestPackageArchiveCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	root := t.TempDir()
	output := filepath.Join(t.TempDir(), "archive.tar.gz")
	var stage string
	_, err := packageArchive(ctx, root, &shop.Config{}, ArchiveOptions{OutputPath: output}, func(_ context.Context, dir string) error {
		stage = dir
		cancel()
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
	assert.NoDirExists(t, filepath.Dir(stage))
	assert.NoFileExists(t, output)
}

func TestPackageArchiveWithTemporaryDirectoryInsideProject(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TMPDIR", root)
	t.Setenv("TMP", root)
	t.Setenv("TEMP", root)
	reference, err := packageArchive(t.Context(), root, &shop.Config{}, ArchiveOptions{}, func(_ context.Context, dir string) error {
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		assert.Empty(t, entries, "must not recursively copy the staging directory")
		return nil
	})
	require.NoError(t, err)
	assert.FileExists(t, reference)
}

func TestPackageArchiveOmitsCustomConfigsAndOverrides(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	for _, name := range []string{"custom.yml", "custom.local.yml", "included.yml", "included.local.yml"} {
		testhelper.WriteFile(t, filepath.Join(root, name), "credentials")
	}
	cfg := &shop.Config{AdditionalConfigs: []string{"included.yml"}}
	reference, err := packageArchive(t.Context(), root, cfg, ArchiveOptions{ConfigPath: "custom.yml"}, func(context.Context, string) error {
		return nil
	})
	require.NoError(t, err)
	assert.Empty(t, readArchive(t, reference))
}

func TestPackageArchivePreservesContainedSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require privileges on Windows")
	}
	root := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(root, "target"), "source")
	require.NoError(t, os.Symlink("target", filepath.Join(root, "link")))
	reference, err := packageArchive(t.Context(), root, &shop.Config{}, ArchiveOptions{}, func(_ context.Context, stage string) error {
		return os.WriteFile(filepath.Join(stage, "link"), []byte("built"), 0o644)
	})
	require.NoError(t, err)
	assert.Equal(t, "built", readArchive(t, reference)["target"])
	source, err := os.ReadFile(filepath.Join(root, "target"))
	require.NoError(t, err)
	assert.Equal(t, "source", string(source))
}
