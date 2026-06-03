package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	assert.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	assert.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestScanLocalPublic(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "public", "media", "ab", "cd", "image.jpg"), "image")
	writeFile(t, filepath.Join(root, "public", "thumbnail", "thumb.jpg"), "thumb")
	// Files outside the datastore prefixes must be ignored.
	writeFile(t, filepath.Join(root, "public", "index.php"), "<?php")
	writeFile(t, filepath.Join(root, "public", "theme", "css", "all.css"), "css")

	ds, _ := DatastoreByName("public")
	result, err := ScanLocal(root, ds)
	assert.NoError(t, err)

	keys := make([]string, 0, len(result.Files))
	for _, f := range result.Files {
		keys = append(keys, f.Key)
	}

	assert.ElementsMatch(t, []string{"media/ab/cd/image.jpg", "thumbnail/thumb.jpg"}, keys)
	assert.EqualValues(t, len("image")+len("thumb"), result.TotalSize)
}

func TestScanLocalPrivateWholeDir(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "files", "documents", "invoice.pdf"), "pdf")

	ds, _ := DatastoreByName("private")
	result, err := ScanLocal(root, ds)
	assert.NoError(t, err)

	assert.Len(t, result.Files, 1)
	assert.Equal(t, "documents/invoice.pdf", result.Files[0].Key)
}

func TestScanLocalMissingDir(t *testing.T) {
	root := t.TempDir()

	ds, _ := DatastoreByName("public")
	result, err := ScanLocal(root, ds)
	assert.NoError(t, err)
	assert.Empty(t, result.Files)
	assert.Zero(t, result.TotalSize)
}

func TestObjectKey(t *testing.T) {
	ds, _ := DatastoreByName("public")

	assert.Equal(t, "media/x.jpg", StoreTarget{Datastore: ds}.objectKey(LocalFile{Key: "media/x.jpg"}))
	assert.Equal(t, "prefix/media/x.jpg", StoreTarget{Datastore: ds, Root: "/prefix/"}.objectKey(LocalFile{Key: "media/x.jpg"}))
}

func TestRenderFilesystemConfigPublic(t *testing.T) {
	public, _ := DatastoreByName("public")
	private, _ := DatastoreByName("private")

	out := RenderFilesystemConfig(ConfigOptions{
		Connection: S3Connection{
			Endpoint:     "https://minio.example.com",
			Region:       "eu-central-1",
			AccessKey:    "AKIARAWACCESS",
			SecretKey:    "RAWSECRETVALUE",
			UsePathStyle: true,
		},
		PublicACL: true,
		Targets: []StoreTarget{
			{Datastore: public, Bucket: "public-bucket", PublicURL: "https://cdn.example.com"},
			{Datastore: private, Bucket: "private-bucket"},
		},
	})

	// Structure and known keys.
	assert.Contains(t, out, "shopware:")
	assert.Contains(t, out, "    filesystem:")
	assert.Contains(t, out, "        public:")
	assert.Contains(t, out, "        private:")
	assert.Contains(t, out, "type: \"amazon-s3\"")
	assert.Contains(t, out, "bucket: \"public-bucket\"")
	assert.Contains(t, out, "url: \"https://cdn.example.com\"")
	assert.Contains(t, out, "use_path_style_endpoint: true")
	assert.Contains(t, out, "endpoint: \"https://minio.example.com\"")

	// Credentials are referenced via env placeholders, never inlined.
	assert.Contains(t, out, "key: \"%env(STORAGE_S3_ACCESS_KEY)%\"")
	assert.Contains(t, out, "secret: \"%env(STORAGE_S3_SECRET_KEY)%\"")
	assert.NotContains(t, out, "AKIARAWACCESS")
	assert.NotContains(t, out, "RAWSECRETVALUE")

	// Public datastore gets a public visibility, private one does not.
	publicIdx := strings.Index(out, "        public:")
	privateIdx := strings.Index(out, "        private:")
	assert.GreaterOrEqual(t, publicIdx, 0)
	assert.Greater(t, privateIdx, publicIdx)

	publicSection := out[publicIdx:privateIdx]
	assert.Contains(t, publicSection, "visibility: \"public\"")
	privateSection := out[privateIdx:]
	assert.NotContains(t, privateSection, "visibility: \"public\"")
}

func TestConfigEnvVars(t *testing.T) {
	opts := ConfigOptions{
		Connection: S3Connection{AccessKey: "ak", SecretKey: "sk"},
	}
	vars := opts.EnvVars()
	assert.Equal(t, "ak", vars[DefaultAccessKeyEnv])
	assert.Equal(t, "sk", vars[DefaultSecretKeyEnv])
}

func TestMigrateDryRun(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "files", "a.txt"), "aaa")
	writeFile(t, filepath.Join(root, "files", "sub", "b.txt"), "bbbbb")

	ds, _ := DatastoreByName("private")
	scan, err := ScanLocal(root, ds)
	assert.NoError(t, err)

	scans := map[string]ScanResult{ds.Name: scan}
	targets := []StoreTarget{{Datastore: ds, Bucket: "bucket"}}

	var events int
	// client may be nil on dry-run: no S3 calls are made.
	summary, err := Migrate(t.Context(), nil, targets, scans, MigrateOptions{DryRun: true}, func(ev ProgressEvent) {
		events++
		assert.NoError(t, ev.Err)
	})
	assert.NoError(t, err)
	assert.Equal(t, 2, summary.Uploaded)
	assert.Equal(t, 0, summary.Failed)
	assert.Equal(t, 2, events)
}
