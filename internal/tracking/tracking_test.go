package tracking

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTrackSendsUDPPayload(t *testing.T) {
	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	assert.NoError(t, err)

	conn, err := net.ListenUDP("udp", udpAddr)
	assert.NoError(t, err)
	defer func() { _ = conn.Close() }()

	originalAddr := addr
	addr = conn.LocalAddr().String()
	defer func() { addr = originalAddr }()

	Track(t.Context(), "test_event", map[string]string{"key": "value"})

	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	assert.NoError(t, err)

	var received event
	err = json.Unmarshal(buf[:n], &received)
	assert.NoError(t, err)

	assert.Equal(t, "shopware_cli.test_event", received.Event)
	assert.Equal(t, "value", received.Tags["key"])
	assert.NotEmpty(t, received.UserID)
	assert.NotEmpty(t, received.Timestamp)
}

func TestTrackRespectsDoNotTrack(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "1")

	// Should return without sending anything
	Track(t.Context(), "should_not_send", nil)
}

// clearCIEnvVars unsets all CI environment variables that resolveID checks,
// so tests run correctly even when executed on CI (e.g. GitHub Actions).
func clearCIEnvVars(t *testing.T) {
	t.Helper()
	t.Setenv("GITHUB_REPOSITORY", "")
	t.Setenv("CI_PROJECT_URL", "")
	t.Setenv("BITBUCKET_REPO_FULL_NAME", "")
}

func TestResolveIDFromGitHub(t *testing.T) {
	clearCIEnvVars(t)
	t.Setenv("GITHUB_REPOSITORY", "shopware/shopware")

	result := resolveID()

	h := sha256.Sum256([]byte("shopware/shopware"))
	expected := hex.EncodeToString(h[:16])
	assert.Equal(t, expected, result)
}

func TestResolveIDFromGitLab(t *testing.T) {
	clearCIEnvVars(t)
	t.Setenv("CI_PROJECT_URL", "https://gitlab.example.com/group/repo")

	result := resolveID()

	h := sha256.Sum256([]byte("https://gitlab.example.com/group/repo"))
	expected := hex.EncodeToString(h[:16])
	assert.Equal(t, expected, result)
}

func TestResolveIDFromBitbucket(t *testing.T) {
	clearCIEnvVars(t)
	t.Setenv("BITBUCKET_REPO_FULL_NAME", "workspace/repo")

	result := resolveID()

	h := sha256.Sum256([]byte("workspace/repo"))
	expected := hex.EncodeToString(h[:16])
	assert.Equal(t, expected, result)
}

func TestResolveIDPersistsLocally(t *testing.T) {
	clearCIEnvVars(t)
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir) // Linux
	t.Setenv("AppData", tmpDir)         // Windows
	t.Setenv("HOME", tmpDir)            // macOS fallback

	// Ensure config dir exists (OS normally provides this)
	configDir, err := os.UserConfigDir()
	assert.NoError(t, err)
	assert.NoError(t, os.MkdirAll(configDir, 0o755))

	// First call generates and persists
	id1 := resolveID()
	assert.Len(t, id1, 32)

	// Check file was created
	idPath := filepath.Join(configDir, idFileName)
	data, err := os.ReadFile(idPath)
	assert.NoError(t, err)
	assert.Equal(t, id1, string(data))

	// Second call reads from file
	id2 := resolveID()
	assert.Equal(t, id1, id2)
}

func TestResolveIDCIPrioritizedOverLocal(t *testing.T) {
	clearCIEnvVars(t)
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Write an existing local ID
	configDir, _ := os.UserConfigDir()
	_ = os.MkdirAll(configDir, 0o755)
	_ = os.WriteFile(filepath.Join(configDir, idFileName), []byte("a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4"), 0o600)

	// Set CI env — should take priority
	t.Setenv("GITHUB_REPOSITORY", "shopware/shopware")

	result := resolveID()

	h := sha256.Sum256([]byte("shopware/shopware"))
	expected := hex.EncodeToString(h[:16])
	assert.Equal(t, expected, result)
}
