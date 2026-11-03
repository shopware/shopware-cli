package tracking

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shopware/shopware-cli/internal/userconfig"
)

// TestMain points the user config at a missing file so tests never read the
// developer's real config.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "shopware-cli-config")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv(userconfig.PathEnv, filepath.Join(dir, "config.yaml")); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
