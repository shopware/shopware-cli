package extension

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidPackageJsonBun(t *testing.T) {
	tmpDir := t.TempDir()

	packageJson := `{
		"dependencies": {
			"foo": "1.0.0"
		}
	}`

	if err := os.WriteFile(path.Join(tmpDir, "package.json"), []byte(packageJson), 0644); err != nil {
		t.Fatal(err)
	}

	npm, err := getNpmPackage(tmpDir)

	assert.NoError(t, err)

	assert.True(t, canRunBunOnPackage(npm))
}

func TestValidPackageJsonWithDevBun(t *testing.T) {
	tmpDir := t.TempDir()

	packageJson := `{
		"dependencies": {
			"foo": "1.0.0"
		},
		"devDependencies": {
			"bar": "1.0.0"
		}
	}`

	if err := os.WriteFile(path.Join(tmpDir, "package.json"), []byte(packageJson), 0644); err != nil {
		t.Fatal(err)
	}

	npm, err := getNpmPackage(tmpDir)

	assert.NoError(t, err)

	assert.True(t, canRunBunOnPackage(npm))
}

func TestInvalidPackageJsonBun(t *testing.T) {
	tmpDir := t.TempDir()

	packageJson := `{
		"dependencies": {
			"foo": "1.0.0"
		},
		"devDependencies": {
			"foo": "1.0.0"
		}
	}`

	if err := os.WriteFile(path.Join(tmpDir, "package.json"), []byte(packageJson), 0644); err != nil {
		t.Fatal(err)
	}

	npm, err := getNpmPackage(tmpDir)

	assert.NoError(t, err)

	assert.False(t, canRunBunOnPackage(npm))
}
