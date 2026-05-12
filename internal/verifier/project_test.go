package verifier

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockPackagistTransport redirects all HTTP requests to a test server, preserving the path.
type mockPackagistTransport struct {
	server *httptest.Server
}

func (m *mockPackagistTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	newReq, err := http.NewRequestWithContext(req.Context(), req.Method, m.server.URL+req.URL.Path, req.Body)
	if err != nil {
		return nil, err
	}
	newReq.Header = req.Header
	return m.server.Client().Transport.RoundTrip(newReq)
}

// mockPackagistAPI installs a fake packagist server for the duration of the test.
// This is required because GetConfigFromProject calls determineVersionRange which
// fetches Shopware versions from repo.packagist.org.
func mockPackagistAPI(t *testing.T) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"packages":{"shopware/core":[{"version_normalized":"6.6.0.0"}]}}`))
	}))
	t.Cleanup(server.Close)

	original := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = original })
	http.DefaultClient = &http.Client{Transport: &mockPackagistTransport{server: server}}
}

const testProjectYAMLSingleBundle = `compatibility_date: "2024-01-01"
build:
  bundles:
    - path: src/MyBundle
`

func TestGetConfigFromProjectYAMLBundles(t *testing.T) {
	mockPackagistAPI(t)
	tmpDir := t.TempDir()

	// Minimal composer.json with shopware/core requirement
	assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "composer.json"), []byte(`{
		"type": "project",
		"require": {"shopware/core": "~6.6.0"}
	}`), 0o644))

	// Create bundle directory with an admin subfolder
	adminPath := filepath.Join(tmpDir, "src", "MyBundle", "Resources", "app", "administration")
	assert.NoError(t, os.MkdirAll(adminPath, os.ModePerm))

	// Write .shopware-project.yml with the bundle declared
	assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".shopware-project.yml"), []byte(testProjectYAMLSingleBundle), 0o644))

	cfg, err := GetConfigFromProject(tmpDir, true)
	assert.NoError(t, err)

	assert.Contains(t, cfg.SourceDirectories, filepath.Join(tmpDir, "src", "MyBundle"))
	assert.Contains(t, cfg.AdminDirectories, adminPath)
}

func TestGetConfigFromProjectYAMLBundleStorefront(t *testing.T) {
	mockPackagistAPI(t)
	tmpDir := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "composer.json"), []byte(`{
		"type": "project",
		"require": {"shopware/core": "~6.6.0"}
	}`), 0o644))

	// Create bundle directory with a storefront subfolder only
	storefrontPath := filepath.Join(tmpDir, "src", "MyBundle", "Resources", "app", "storefront")
	assert.NoError(t, os.MkdirAll(storefrontPath, os.ModePerm))

	assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".shopware-project.yml"), []byte(testProjectYAMLSingleBundle), 0o644))

	cfg, err := GetConfigFromProject(tmpDir, true)
	assert.NoError(t, err)

	assert.Contains(t, cfg.SourceDirectories, filepath.Join(tmpDir, "src", "MyBundle"))
	assert.Contains(t, cfg.StorefrontDirectories, storefrontPath)
}

func TestGetConfigFromProjectYAMLBundleDeduplication(t *testing.T) {
	mockPackagistAPI(t)
	tmpDir := t.TempDir()

	// composer.json declares the same bundle
	assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "composer.json"), []byte(`{
		"type": "project",
		"require": {"shopware/core": "~6.6.0"},
		"extra": {"shopware-bundles": {"src/MyBundle": {"name": "MyBundle"}}}
	}`), 0o644))

	assert.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "src", "MyBundle"), os.ModePerm))

	assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".shopware-project.yml"), []byte(testProjectYAMLSingleBundle), 0o644))

	cfg, err := GetConfigFromProject(tmpDir, true)
	assert.NoError(t, err)

	bundleSrcPath := filepath.Join(tmpDir, "src", "MyBundle")
	count := 0
	for _, d := range cfg.SourceDirectories {
		if d == bundleSrcPath {
			count++
		}
	}
	assert.Equal(t, 1, count, "bundle declared in both composer.json and YAML config should only appear once in SourceDirectories")
}
