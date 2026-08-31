// Package testhelper provides shared fixture builders for tests: writing
// files into temp directories and rendering the composer.json / composer.lock
// shapes that Shopware project and extension tests need over and over.
package testhelper

import (
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// WriteFile writes content to path, creating parent directories as needed.
func WriteFile(tb testing.TB, filePath, content string) {
	tb.Helper()
	require.NoError(tb, os.MkdirAll(filepath.Dir(filePath), 0o755))
	require.NoError(tb, os.WriteFile(filePath, []byte(content), 0o644))
}

// ExtensionDir writes composer.json into a fresh temp directory and returns
// the directory: a standalone plugin or bundle root. Use it when the test
// cares about the composer content — the manifest stays visible at the call
// site; reach for NewPlugin when any valid plugin will do.
func ExtensionDir(tb testing.TB, c ComposerJSON) string {
	tb.Helper()
	dir := tb.TempDir()
	WriteFile(tb, filepath.Join(dir, "composer.json"), c.String())
	return dir
}

// AppDir writes manifest.xml into a fresh temp directory and returns the
// directory: a standalone Shopware app root.
func AppDir(tb testing.TB, m AppManifest) string {
	tb.Helper()
	dir := tb.TempDir()
	WriteFile(tb, filepath.Join(dir, "manifest.xml"), m.String())
	return dir
}

// NewPlugin returns a directory containing a valid platform plugin for tests
// that just need one and don't assert its metadata. The composer name and
// bootstrap class are derived from name: "FroshTools" becomes
// "test/frosh-tools" with class `FroshTools\FroshTools` at version 1.0.0.
// Tests that assert those values should use ExtensionDir with an explicit
// ComposerJSON instead.
func NewPlugin(tb testing.TB, name string) string {
	tb.Helper()
	return ExtensionDir(tb, PluginComposer("test/"+kebabCase(name), "1.0.0", name+`\`+name))
}

// NewApp returns a directory containing a valid app manifest for tests that
// just need one and don't assert its metadata.
func NewApp(tb testing.TB, name string) string {
	tb.Helper()
	return AppDir(tb, NewAppManifest(name))
}

// Project is a Shopware project rooted in a temp directory that tests can
// populate file by file.
type Project struct {
	TB   testing.TB
	Root string
}

// NewProject creates an empty project in a fresh temp directory.
func NewProject(tb testing.TB) *Project {
	tb.Helper()
	return &Project{TB: tb, Root: tb.TempDir()}
}

// Dir creates an empty directory at the slash-separated path relative to the
// project root.
func (p *Project) Dir(rel string) *Project {
	p.TB.Helper()
	require.NoError(p.TB, os.MkdirAll(filepath.Join(p.Root, filepath.FromSlash(rel)), 0o755))
	return p
}

// File writes content at the slash-separated path relative to the project root.
func (p *Project) File(rel, content string) *Project {
	p.TB.Helper()
	WriteFile(p.TB, filepath.Join(p.Root, filepath.FromSlash(rel)), content)
	return p
}

// VendorPackage writes vendor/<name>/composer.json for a Composer-managed package.
func (p *Project) VendorPackage(name string, composer ComposerJSON) *Project {
	p.TB.Helper()
	return p.File(path.Join("vendor", name, "composer.json"), composer.String())
}

// CustomPlugin writes custom/plugins/<dirName>/composer.json for a locally
// installed plugin.
func (p *Project) CustomPlugin(dirName string, composer ComposerJSON) *Project {
	p.TB.Helper()
	return p.File(path.Join("custom", "plugins", dirName, "composer.json"), composer.String())
}
