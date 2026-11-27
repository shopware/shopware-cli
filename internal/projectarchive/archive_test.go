package projectarchive

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/sqlshell"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func localFileRoots(root string) FileRoots {
	return FileRoots{public: filepath.Join(root, "public"), private: filepath.Join(root, "files")}
}

// sharedDir links project/<name> to shared/<name>, as SSH deployments do.
func sharedDir(t *testing.T, root, name string) {
	t.Helper()
	target := filepath.Join(root, "shared", name)
	require.NoError(t, os.MkdirAll(target, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, "project", name)), 0o755))
	require.NoError(t, os.Symlink(target, filepath.Join(root, "project", name)))
}

func TestExportImportFilesRoundTrip(t *testing.T) {
	files := Components{PublicFiles: true, PrivateFiles: true}

	source := t.TempDir()
	sharedDir(t, source, "public/media")
	writeFile(t, filepath.Join(source, "shared/public/media/a/logo.png"), "logo")
	writeFile(t, filepath.Join(source, "project/public/thumbnail/a/logo_400x400.png"), "thumb")
	writeFile(t, filepath.Join(source, "project/public/theme/all.css"), "not exported")
	writeFile(t, filepath.Join(source, "project/files/documents/invoice.pdf"), "invoice")

	// Named like a payload entry, which must not clash with the staged one.
	archive := filepath.Join(t.TempDir(), privateFilesEntry)
	sourceProject := filepath.Join(source, "project")
	require.NoError(t, Export(t.Context(), ExportOptions{
		Output:          archive,
		Components:      files,
		Environment:     executor.NewLocal(sourceProject),
		FileRoots:       localFileRoots(sourceProject),
		ShopwareVersion: "6.7.1.0",
	}))

	info, err := os.Stat(archive)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "archive may contain sensitive data")

	contents, err := Inspect(archive)
	require.NoError(t, err)
	assert.Equal(t, files, contents.Components)
	assert.Equal(t, FormatVersion, contents.Metadata.FormatVersion)
	assert.Equal(t, "6.7.1.0", contents.Metadata.Source.ShopwareVersion)
	assert.Nil(t, contents.Metadata.Source.Database)

	target := t.TempDir()
	sharedDir(t, target, "public/media")
	sharedDir(t, target, "public/thumbnail")
	sharedDir(t, target, "files")
	writeFile(t, filepath.Join(target, "shared/public/media/stale.png"), "stale")
	writeFile(t, filepath.Join(target, "shared/files/.hidden"), "stale")
	writeFile(t, filepath.Join(target, "project/public/theme/all.css"), "kept")

	// SSH deployments seal the release, only the shared directories are writable.
	sealed := filepath.Join(target, "project/public")
	require.NoError(t, os.Chmod(sealed, 0o555))
	t.Cleanup(func() { _ = os.Chmod(sealed, 0o755) })

	targetProject := filepath.Join(target, "project")
	require.NoError(t, Import(t.Context(), ImportOptions{
		Archive:     archive,
		Contents:    contents,
		Environment: executor.NewLocal(targetProject),
		FileRoots:   localFileRoots(targetProject),
	}))

	assert.FileExists(t, filepath.Join(target, "shared/public/media/a/logo.png"))
	assert.FileExists(t, filepath.Join(target, "shared/public/thumbnail/a/logo_400x400.png"))
	assert.FileExists(t, filepath.Join(target, "shared/files/documents/invoice.pdf"))
	assert.NoFileExists(t, filepath.Join(target, "shared/public/media/stale.png"))
	assert.NoFileExists(t, filepath.Join(target, "shared/files/.hidden"))
	assert.FileExists(t, filepath.Join(target, "project/public/theme/all.css"))

	link, err := os.Lstat(filepath.Join(target, "project/public/media"))
	require.NoError(t, err)
	assert.NotZero(t, link.Mode()&os.ModeSymlink, "shared directory symlink must survive the import")
}

func TestExportSkipsMissingDirectories(t *testing.T) {
	// A sealed release without media, thumbnails or a private filesystem.
	project := t.TempDir()
	public := filepath.Join(project, "public")
	require.NoError(t, os.MkdirAll(public, 0o555))
	t.Cleanup(func() { _ = os.Chmod(public, 0o755) })

	archive := filepath.Join(t.TempDir(), "archive.tar.gz")
	require.NoError(t, Export(t.Context(), ExportOptions{
		Output:          archive,
		Components:      Components{PublicFiles: true, PrivateFiles: true},
		Environment:     executor.NewLocal(project),
		FileRoots:       localFileRoots(project),
		ShopwareVersion: "6.7.1.0",
	}))

	assert.NoDirExists(t, filepath.Join(project, "files"), "export must not write to the source")

	contents, err := Inspect(archive)
	require.NoError(t, err)
	assert.Equal(t, Components{PublicFiles: true, PrivateFiles: true}, contents.Components)
}

// openExecer returns an ImportOptions.OpenDatabase that opens execer.
func openExecer(execer sqlshell.Execer) func(context.Context) (sqlshell.Execer, func(), error) {
	return func(context.Context) (sqlshell.Execer, func(), error) {
		return execer, func() {}, nil
	}
}

// noopExecer accepts every statement of a database import.
type noopExecer struct{}

func (noopExecer) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return driver.RowsAffected(0), nil
}

func TestImportEmptiesFilesMissingFromArchive(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "archive.tar.gz")
	writeTestArchive(t, archive, map[string]string{
		metadataEntry: `{"format_version": 1, "source": {"shopware_version": "6.7.1.0"}}`,
		databaseEntry: "SELECT 1;",
	})

	project := t.TempDir()
	writeFile(t, filepath.Join(project, "public/media/a/stale.png"), "stale")
	writeFile(t, filepath.Join(project, "public/thumbnail/a/stale_400x400.png"), "stale")
	writeFile(t, filepath.Join(project, "public/theme/all.css"), "kept")
	writeFile(t, filepath.Join(project, "files/documents/invoice.pdf"), "stale")

	contents, err := Inspect(archive)
	require.NoError(t, err)

	require.NoError(t, Import(t.Context(), ImportOptions{
		Archive:      archive,
		Contents:     contents,
		Environment:  executor.NewLocal(project),
		FileRoots:    localFileRoots(project),
		OpenDatabase: openExecer(noopExecer{}),
	}))

	assert.NoDirExists(t, filepath.Join(project, "public/media/a"))
	assert.NoDirExists(t, filepath.Join(project, "public/thumbnail/a"))
	assert.DirExists(t, filepath.Join(project, "public/media"))
	assert.FileExists(t, filepath.Join(project, "public/theme/all.css"))
	assert.NoFileExists(t, filepath.Join(project, "files/documents/invoice.pdf"))
}

func TestImportAdoptsTargetPermissions(t *testing.T) {
	dir := t.TempDir()
	publicArchive := filepath.Join(dir, publicFilesEntry)
	writeTestArchive(t, publicArchive, map[string]string{"media/a/new.png": "new", "thumbnail/a/new.png": "new"})
	publicContent, err := os.ReadFile(publicArchive)
	require.NoError(t, err)

	archive := filepath.Join(dir, "archive.tar.gz")
	writeTestArchive(t, archive, map[string]string{
		metadataEntry:    `{"format_version": 1, "source": {"shopware_version": "6.7.1.0"}}`,
		publicFilesEntry: string(publicContent),
	})

	// The web server shares the group, so the target is group-writable.
	project := t.TempDir()
	media := filepath.Join(project, "public/media")
	writeFile(t, filepath.Join(media, "old.png"), "old")
	require.NoError(t, os.Chmod(filepath.Join(media, "old.png"), 0o660))
	require.NoError(t, os.Chmod(media, 0o770))
	thumbnail := filepath.Join(project, "public/thumbnail")
	require.NoError(t, os.MkdirAll(thumbnail, 0o750))
	require.NoError(t, os.Chmod(thumbnail, 0o750))

	contents, err := Inspect(archive)
	require.NoError(t, err)

	require.NoError(t, Import(t.Context(), ImportOptions{
		Archive:     archive,
		Contents:    contents,
		Environment: executor.NewLocal(project),
		FileRoots:   localFileRoots(project),
	}))

	assertMode := func(path string, want os.FileMode) {
		t.Helper()
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, want, info.Mode().Perm(), path)
	}
	// Modes of an existing file and of the directory.
	assertMode(filepath.Join(media, "a/new.png"), 0o660)
	assertMode(filepath.Join(media, "a"), 0o770)
	// Without an existing file, the directory's mode without execute bits.
	assertMode(filepath.Join(thumbnail, "a/new.png"), 0o640)
	assertMode(filepath.Join(thumbnail, "a"), 0o750)
}

func TestImportFailsBeforeDatabaseWhenFilesCannotBeRemoved(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can remove every file")
	}

	dir := t.TempDir()
	publicArchive := filepath.Join(dir, publicFilesEntry)
	writeTestArchive(t, publicArchive, map[string]string{"media/a/new.png": "new"})
	publicContent, err := os.ReadFile(publicArchive)
	require.NoError(t, err)

	archive := filepath.Join(dir, "archive.tar.gz")
	writeTestArchive(t, archive, map[string]string{
		metadataEntry:    `{"format_version": 1, "source": {"shopware_version": "6.7.1.0"}}`,
		databaseEntry:    "SELECT 1;",
		publicFilesEntry: string(publicContent),
	})

	// A subdirectory of another user, e.g. the web server, that the importing
	// user may not change.
	project := t.TempDir()
	writeFile(t, filepath.Join(project, "public/media/b/old.png"), "old")
	foreign := filepath.Join(project, "public/media/b")
	require.NoError(t, os.Chmod(foreign, 0o555))
	t.Cleanup(func() { _ = os.Chmod(foreign, 0o755) })

	contents, err := Inspect(archive)
	require.NoError(t, err)

	err = Import(t.Context(), ImportOptions{
		Archive:     archive,
		Contents:    contents,
		Environment: executor.NewLocal(project),
		FileRoots:   localFileRoots(project),
		OpenDatabase: func(context.Context) (sqlshell.Execer, func(), error) {
			t.Error("the database must not be touched")
			return noopExecer{}, func() {}, nil
		},
	})
	require.Error(t, err)

	assert.FileExists(t, filepath.Join(project, "public/media/b/old.png"))
	matches, err := filepath.Glob(filepath.Join(project, "public/media", stagingPrefix+"*"))
	require.NoError(t, err)
	assert.Empty(t, matches, "staging directories must be removed")
}

// failingExecer rejects every statement of a database import.
type failingExecer struct{}

func (failingExecer) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, errors.New("connection lost")
}

func TestImportKeepsFilesWhenDatabaseImportFails(t *testing.T) {
	dir := t.TempDir()
	publicArchive := filepath.Join(dir, publicFilesEntry)
	writeTestArchive(t, publicArchive, map[string]string{"media/a/new.png": "new"})
	publicContent, err := os.ReadFile(publicArchive)
	require.NoError(t, err)

	archive := filepath.Join(dir, "archive.tar.gz")
	writeTestArchive(t, archive, map[string]string{
		metadataEntry:    `{"format_version": 1, "source": {"shopware_version": "6.7.1.0"}}`,
		databaseEntry:    "SELECT 1;",
		publicFilesEntry: string(publicContent),
	})

	project := t.TempDir()
	writeFile(t, filepath.Join(project, "public/media/a/old.png"), "old")
	writeFile(t, filepath.Join(project, "files/documents/invoice.pdf"), "old")

	contents, err := Inspect(archive)
	require.NoError(t, err)

	err = Import(t.Context(), ImportOptions{
		Archive:      archive,
		Contents:     contents,
		Environment:  executor.NewLocal(project),
		FileRoots:    localFileRoots(project),
		OpenDatabase: openExecer(failingExecer{}),
	})
	require.Error(t, err)

	assert.FileExists(t, filepath.Join(project, "public/media/a/old.png"))
	assert.NoFileExists(t, filepath.Join(project, "public/media/a/new.png"))
	assert.FileExists(t, filepath.Join(project, "files/documents/invoice.pdf"))

	for _, staging := range []string{"public/media", "files"} {
		matches, err := filepath.Glob(filepath.Join(project, staging, stagingPrefix+"*"))
		require.NoError(t, err)
		assert.Empty(t, matches, "staging directories must be removed")
	}
}

func TestInspectValidatesArchive(t *testing.T) {
	validMetadata := `{"format_version": 1, "created_at": "2026-08-11T12:00:00Z", "source": {"shopware_version": "6.7.1.0"}, "unknown": true}`

	cases := []struct {
		name    string
		entries map[string]string
		wantErr string
	}{
		{name: "valid", entries: map[string]string{metadataEntry: validMetadata, databaseEntry: "SELECT 1;"}},
		{name: "missing metadata", entries: map[string]string{databaseEntry: "SELECT 1;"}, wantErr: "metadata.json is missing"},
		{name: "unknown format version", entries: map[string]string{metadataEntry: `{"format_version": 2}`, databaseEntry: ""}, wantErr: "unsupported format version 2"},
		{name: "two database payloads", entries: map[string]string{metadataEntry: validMetadata, databaseEntry: "", "database/@.json": "{}"}, wantErr: "both"},
		{name: "no components", entries: map[string]string{metadataEntry: validMetadata}, wantErr: "neither"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "archive.tar.gz")
			writeTestArchive(t, path, tc.entries)

			contents, err := Inspect(path)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, Components{Database: true}, contents.Components)
		})
	}
}

func writeTestArchive(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	file, err := os.Create(path)
	require.NoError(t, err)
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	for name, content := range entries {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	require.NoError(t, file.Close())
}

func TestParseFileRoots(t *testing.T) {
	// Notices with braces may precede the JSON.
	output := `Deprecated: Method {closure}() is deprecated
{
    "private": {
        "type": "local",
        "config": {"root": "%s"},
        "visibility": "private"
    },
    "public": {
        "type": "amazon-s3",
        "url": "https://cdn.example.com",
        "config": {"bucket": "media"}
    }
}`

	cases := []struct {
		name     string
		root     string
		needed   Components
		wantRoot string
		wantErr  string
	}{
		{name: "local filesystem", root: "/var/www/html/files", needed: Components{PrivateFiles: true}, wantRoot: "/var/www/html/files"},
		{name: "unsupported adapter", root: "/var/www/html/files", needed: Components{PublicFiles: true, PrivateFiles: true}, wantErr: "amazon-s3"},
		{name: "unresolved root", root: "%%env(PRIVATE_ROOT)%%", needed: Components{PrivateFiles: true}, wantErr: "unresolved"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			roots, err := parseFileRoots([]byte(fmt.Sprintf(output, tc.root)), tc.needed)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			// The unsupported public filesystem is fine, as it is not needed.
			assert.Equal(t, FileRoots{private: tc.wantRoot}, roots)
		})
	}
}

func TestParseShopwareVersion(t *testing.T) {
	version, err := parseShopwareVersion([]byte("Deprecated: something\n{\n    \"kernel.shopware_version\": \"6.7.1.0\"\n}\nDeprecated: {closure}\n"))
	require.NoError(t, err)
	assert.Equal(t, "6.7.1.0", version)

	_, err = parseShopwareVersion([]byte(`{"kernel.shopware_version": ""}`))
	assert.Error(t, err)
}
