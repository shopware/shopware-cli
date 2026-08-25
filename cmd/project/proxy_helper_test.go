package project

import (
	"testing"
)

// isolateProxyState points the proxy state directory (registry.json and
// settings.json, resolved through os.UserConfigDir) at a temp dir.
func isolateProxyState(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
}
