package projectarchive

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"database/sql/driver"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/executor"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func localFilesystems(root string) Filesystems {
	var fs Filesystems
	fs.Public.Type = localAdapter
	fs.Public.Config.Root = filepath.Join(root, "public")
	fs.Private.Type = localAdapter
	fs.Private.Config.Root = filepath.Join(root, "files")
	return fs
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
		Filesystems:     localFilesystems(sourceProject),
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
		Components:  files,
		Environment: executor.NewLocal(targetProject),
		Filesystems: localFilesystems(targetProject),
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
	writeFile(t, filepath.Join(project, "files/documents/invoice.pdf"), "kept")

	require.NoError(t, Import(t.Context(), ImportOptions{
		Archive:     archive,
		Components:  Components{Database: true},
		EmptyFiles:  Components{PublicFiles: true},
		Environment: executor.NewLocal(project),
		Filesystems: localFilesystems(project),
		Database:    noopExecer{},
	}))

	assert.NoDirExists(t, filepath.Join(project, "public/media/a"))
	assert.NoDirExists(t, filepath.Join(project, "public/thumbnail/a"))
	assert.DirExists(t, filepath.Join(project, "public/media"))
	assert.FileExists(t, filepath.Join(project, "public/theme/all.css"))
	assert.FileExists(t, filepath.Join(project, "files/documents/invoice.pdf"))
}

func TestImportRejectsUnsupportedFilesystemBeforeWriting(t *testing.T) {
	cases := []struct {
		name    string
		modify  func(fs *Filesystem)
		wantErr string
	}{
		{name: "non-local adapter", modify: func(fs *Filesystem) { fs.Type = "amazon-s3" }, wantErr: "amazon-s3"},
		{name: "unresolved root", modify: func(fs *Filesystem) { fs.Config.Root = "%env(PRIVATE_ROOT)%" }, wantErr: "unresolved"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			project := t.TempDir()
			writeFile(t, filepath.Join(project, "public/media/keep.png"), "kept")

			fs := localFilesystems(project)
			tc.modify(&fs.Private)

			err := Import(t.Context(), ImportOptions{
				Archive:     filepath.Join(t.TempDir(), "missing.tar.gz"),
				Components:  Components{PublicFiles: true, PrivateFiles: true},
				Environment: executor.NewLocal(project),
				Filesystems: fs,
			})

			require.ErrorContains(t, err, tc.wantErr)
			assert.FileExists(t, filepath.Join(project, "public/media/keep.png"))
		})
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

func TestParseFilesystems(t *testing.T) {
	output := `
Current configuration for "shopware.filesystem"
===============================================

private:
    type: local
    config:
        root: /var/www/html/files
    visibility: private
public:
    type: amazon-s3
    url: 'https://cdn.example.com'
    config:
        bucket: media
`

	fs, err := parseFilesystems([]byte(output))
	require.NoError(t, err)
	assert.Equal(t, "local", fs.Private.Type)
	assert.Equal(t, "/var/www/html/files", fs.Private.Config.Root)
	assert.Equal(t, "amazon-s3", fs.Public.Type)
}

func TestParseShopwareVersion(t *testing.T) {
	version, err := parseShopwareVersion([]byte("Deprecated: something\n{\n    \"kernel.shopware_version\": \"6.7.1.0\"\n}\n"))
	require.NoError(t, err)
	assert.Equal(t, "6.7.1.0", version)

	_, err = parseShopwareVersion([]byte(`{"kernel.shopware_version": ""}`))
	assert.Error(t, err)
}
