package proxy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUpdateProjectConfigURLCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".shopware-project.yml")

	assert.NoError(t, UpdateProjectConfigURL(path, "https://my-shop.127.0.0.1.sslip.io"))

	content, err := os.ReadFile(path)
	assert.NoError(t, err)
	assert.Equal(t, "url: https://my-shop.127.0.0.1.sslip.io\n", string(content))
}

func TestUpdateProjectConfigURLUpdatesExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".shopware-project.yml")

	existing := `# my project
url: http://localhost:8000
admin_api:
  username: admin
  password: shopware
`
	assert.NoError(t, os.WriteFile(path, []byte(existing), 0o600))

	assert.NoError(t, UpdateProjectConfigURL(path, "https://my-shop.127.0.0.1.sslip.io"))

	content, err := os.ReadFile(path)
	assert.NoError(t, err)
	assert.Contains(t, string(content), "url: https://my-shop.127.0.0.1.sslip.io")
	assert.Contains(t, string(content), "# my project")
	assert.Contains(t, string(content), "username: admin")
	assert.NotContains(t, string(content), "localhost:8000")
}

func TestUpdateProjectConfigURLAddsMissingKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".shopware-project.yml")

	assert.NoError(t, os.WriteFile(path, []byte("admin_api:\n  username: admin\n  password: shopware\n"), 0o600))

	assert.NoError(t, UpdateProjectConfigURL(path, "https://my-shop.127.0.0.1.sslip.io"))

	content, err := os.ReadFile(path)
	assert.NoError(t, err)
	assert.Contains(t, string(content), "url: https://my-shop.127.0.0.1.sslip.io")
	assert.Contains(t, string(content), "username: admin")
}
