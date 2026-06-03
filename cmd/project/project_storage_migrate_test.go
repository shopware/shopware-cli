package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/storage"
)

func TestUpsertEnvLocalCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env.local")

	err := upsertEnvLocal(path, map[string]string{
		"STORAGE_S3_ACCESS_KEY": "ak",
		"STORAGE_S3_SECRET_KEY": "sk",
	})
	assert.NoError(t, err)

	data, err := os.ReadFile(path)
	assert.NoError(t, err)
	assert.Contains(t, string(data), "STORAGE_S3_ACCESS_KEY=ak\n")
	assert.Contains(t, string(data), "STORAGE_S3_SECRET_KEY=sk\n")
}

func TestUpsertEnvLocalReplacesExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env.local")
	assert.NoError(t, os.WriteFile(path, []byte("APP_ENV=prod\nSTORAGE_S3_ACCESS_KEY=old\n"), 0o644))

	err := upsertEnvLocal(path, map[string]string{"STORAGE_S3_ACCESS_KEY": "new"})
	assert.NoError(t, err)

	data, err := os.ReadFile(path)
	assert.NoError(t, err)
	content := string(data)

	assert.Contains(t, content, "APP_ENV=prod\n")
	assert.Contains(t, content, "STORAGE_S3_ACCESS_KEY=new\n")
	assert.NotContains(t, content, "STORAGE_S3_ACCESS_KEY=old")
}

func TestGuessPublicURL(t *testing.T) {
	// Path-style endpoint -> endpoint/bucket.
	assert.Equal(t, "http://localhost:9000/my-bucket", guessPublicURL(storage.S3Connection{
		Endpoint:     "http://localhost:9000/",
		UsePathStyle: true,
	}, "my-bucket"))

	// Virtual-hosted endpoint -> bucket.host.
	assert.Equal(t, "https://my-bucket.fra1.digitaloceanspaces.com", guessPublicURL(storage.S3Connection{
		Endpoint: "https://fra1.digitaloceanspaces.com",
	}, "my-bucket"))

	// AWS without endpoint -> regional virtual-hosted URL.
	assert.Equal(t, "https://my-bucket.s3.eu-central-1.amazonaws.com", guessPublicURL(storage.S3Connection{
		Region: "eu-central-1",
	}, "my-bucket"))
}
