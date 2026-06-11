package symfony

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConvertXMLFileFixtures copies every testdata/files/<name>/input tree
// into a temporary directory, converts its services.xml or routes.xml and
// compares the resulting tree with testdata/files/<name>/expected.
func TestConvertXMLFileFixtures(t *testing.T) {
	fixturesDir := filepath.Join("testdata", "files")

	entries, err := os.ReadDir(fixturesDir)
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		t.Run(entry.Name(), func(t *testing.T) {
			tmpDir := t.TempDir()
			require.NoError(t, os.CopyFS(tmpDir, os.DirFS(filepath.Join(fixturesDir, entry.Name(), "input"))))

			entryFile := filepath.Join(tmpDir, "services.xml")
			convert := ConvertServicesXMLFile

			if _, err := os.Stat(entryFile); err != nil {
				entryFile = filepath.Join(tmpDir, "routes.xml")
				convert = ConvertRoutesXMLFile
			}

			converted, err := convert(entryFile)
			require.NoError(t, err)
			require.NotEmpty(t, converted)

			for xmlPath, yamlPath := range converted {
				assert.True(t, strings.HasSuffix(xmlPath, ".xml"), xmlPath)
				assert.NoFileExists(t, xmlPath)
				assert.FileExists(t, yamlPath)
			}

			assertSameFiles(t, filepath.Join(fixturesDir, entry.Name(), "expected"), tmpDir)
		})
	}
}

// assertSameFiles checks that gotDir contains exactly the files of wantDir
// with identical content.
func assertSameFiles(t *testing.T, wantDir string, gotDir string) {
	t.Helper()

	wantFiles := listFiles(t, wantDir)
	gotFiles := listFiles(t, gotDir)
	assert.ElementsMatch(t, wantFiles, gotFiles)

	for _, file := range wantFiles {
		want, err := os.ReadFile(filepath.Join(wantDir, file))
		require.NoError(t, err)

		got, err := os.ReadFile(filepath.Join(gotDir, file))
		if assert.NoError(t, err, file) {
			assert.Equal(t, string(want), string(got), file)
		}
	}
}

func listFiles(t *testing.T, root string) []string {
	t.Helper()

	files := []string{}

	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		files = append(files, relPath)

		return nil
	}))

	return files
}

func TestConvertServicesXMLFileRefusesWhenYamlExists(t *testing.T) {
	tmpDir := t.TempDir()
	servicesXML := filepath.Join(tmpDir, "services.xml")

	require.NoError(t, os.WriteFile(servicesXML, []byte(`<container/>`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "services.yaml"), []byte("services:\n"), 0o644))

	_, err := ConvertServicesXMLFile(servicesXML)
	assert.ErrorContains(t, err, "exists already")

	assert.FileExists(t, servicesXML)
}

func TestConvertServicesXMLFileRefusesWhenYmlExists(t *testing.T) {
	tmpDir := t.TempDir()
	servicesXML := filepath.Join(tmpDir, "services.xml")

	require.NoError(t, os.WriteFile(servicesXML, []byte(`<container/>`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "services.yml"), []byte("services:\n"), 0o644))

	_, err := ConvertServicesXMLFile(servicesXML)
	assert.ErrorContains(t, err, "exists already")

	assert.FileExists(t, servicesXML)
}

func TestConvertServicesXMLFileKeepsEverythingOnError(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "packages"), 0o755))

	servicesXML := filepath.Join(tmpDir, "services.xml")
	importedXML := filepath.Join(tmpDir, "packages", "broken.xml")

	require.NoError(t, os.WriteFile(servicesXML, []byte(`<container>
    <imports>
        <import resource="packages/broken.xml"/>
    </imports>
</container>`), 0o644))

	// The imported file contains an element the converter does not support.
	require.NoError(t, os.WriteFile(importedXML, []byte(`<container>
    <services>
        <stack id="foo"/>
    </services>
</container>`), 0o644))

	_, err := ConvertServicesXMLFile(servicesXML)
	assert.ErrorContains(t, err, "unsupported element <stack>")

	assert.FileExists(t, servicesXML)
	assert.FileExists(t, importedXML)
	assert.NoFileExists(t, filepath.Join(tmpDir, "services.yaml"))
	assert.NoFileExists(t, filepath.Join(tmpDir, "packages", "broken.yaml"))
}

func TestConvertServicesXMLFileUnparsableXML(t *testing.T) {
	tmpDir := t.TempDir()
	servicesXML := filepath.Join(tmpDir, "services.xml")

	require.NoError(t, os.WriteFile(servicesXML, []byte(`<container`), 0o644))

	_, err := ConvertServicesXMLFile(servicesXML)
	assert.Error(t, err)
	assert.FileExists(t, servicesXML)
}
