package validation

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeSourcePathStripsTempRoots(t *testing.T) {
	t.Parallel()

	root := "/var/folders/xx/T/extension12345"
	assert.Equal(t, "src/Subscriber/ExampleSubscriber.php", NormalizeSourcePath(root+"/src/Subscriber/ExampleSubscriber.php", root))
	assert.Equal(t, "composer.json", NormalizeSourcePath(root+"/composer.json", root))
	assert.Equal(t, "src/Subscriber/ExampleSubscriber.php", NormalizeSourcePath("/private"+root+"/src/Subscriber/ExampleSubscriber.php", root))
	assert.Equal(t, "composer.json", NormalizeSourcePath("/private"+root+"/composer.json", "/private"+root))
}

func TestNormalizeSourcePathKeepsRelativePaths(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "src/Service.php", NormalizeSourcePath("src/Service.php", "/tmp/extension"))
	assert.Equal(t, "composer.json", NormalizeSourcePath("./composer.json", "/tmp/extension"))
	assert.Equal(t, ".", NormalizeSourcePath("/tmp/extension", "/tmp/extension"))
}

func TestNormalizeSourcePathUsesRealDirectory(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "src", "Service.php")

	assert.Equal(t, "src/Service.php", NormalizeSourcePath(source, root))
	assert.Equal(t, "src/Service.php", NormalizeSourcePath(source, ResolveSourceRoot(root)))
}

func TestNormalizeResultRewritesMessageAndMissingLine(t *testing.T) {
	t.Parallel()

	root := "/tmp/extension12345"
	result := NormalizeResult(CheckResult{
		Path:       root + "/src/Resources/config/services.xml",
		Message:    "Found deprecated " + root + "/src/Resources/config/services.xml. Migrate to yaml.",
		Identifier: "config.services_xml.deprecated",
		Severity:   SeverityWarning,
	}, root)

	assert.Equal(t, "src/Resources/config/services.xml", result.Path)
	assert.Equal(t, "Found deprecated src/Resources/config/services.xml. Migrate to yaml.", result.Message)
	assert.Equal(t, 1, result.Line)
}

func TestNormalizeResultKeepsExistingLine(t *testing.T) {
	t.Parallel()

	result := NormalizeResult(CheckResult{
		Path: "/tmp/extension/src/Service.php",
		Line: 24,
	}, "/tmp/extension")

	assert.Equal(t, "src/Service.php", result.Path)
	assert.Equal(t, 24, result.Line)
}

func TestStripRootFromTextIgnoresSimilarPrefixes(t *testing.T) {
	t.Parallel()

	text := StripRootFromText("kept /tmp/extension-extra/src/Foo.php", "/tmp/extension")
	assert.Contains(t, text, "/tmp/extension-extra/src/Foo.php")
}

func TestResolveSourceRootAbsolutesExistingDir(t *testing.T) {
	root := t.TempDir()

	resolved := ResolveSourceRoot(root)
	require.True(t, filepath.IsAbs(resolved))
	assert.Equal(t, "composer.json", NormalizeSourcePath(filepath.Join(root, "composer.json"), resolved))
}
