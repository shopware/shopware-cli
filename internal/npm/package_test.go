package npm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadPackage(t *testing.T) {
	tmpDir := t.TempDir()

	packageJson := `{
		"dependencies": {
			"foo": "1.0.0"
		}
	}`

	if err := os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(packageJson), 0644); err != nil {
		t.Fatal(err)
	}

	pkg, err := ReadPackage(tmpDir)

	assert.NoError(t, err)
	assert.Equal(t, "1.0.0", pkg.Dependencies["foo"])
}

func TestHasScript(t *testing.T) {
	pkg := Package{
		Scripts: map[string]string{
			"build": "webpack",
		},
	}

	assert.True(t, pkg.HasScript("build"))
	assert.False(t, pkg.HasScript("test"))
}

func TestHasDevDependency(t *testing.T) {
	pkg := Package{
		DevDependencies: map[string]string{
			"puppeteer": "1.0.0",
		},
	}

	assert.True(t, pkg.HasDevDependency("puppeteer"))
	assert.False(t, pkg.HasDevDependency("jest"))
}

func TestNodeModulesExists(t *testing.T) {
	tmpDir := t.TempDir()

	assert.False(t, NodeModulesExists(tmpDir))

	if err := os.Mkdir(filepath.Join(tmpDir, "node_modules"), 0755); err != nil {
		t.Fatal(err)
	}

	assert.True(t, NodeModulesExists(tmpDir))
}
