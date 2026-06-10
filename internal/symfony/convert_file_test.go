package symfony

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertServicesXMLFile(t *testing.T) {
	tmpDir := t.TempDir()
	servicesXML := filepath.Join(tmpDir, "services.xml")

	require.NoError(t, os.WriteFile(servicesXML, []byte(`<container>
    <services>
        <service id="Shop\Service" class="Shop\Service">
            <argument type="service" id="logger"/>
        </service>
    </services>
</container>`), 0o644))

	converted, err := ConvertServicesXMLFile(servicesXML)
	require.NoError(t, err)

	servicesYAML := filepath.Join(tmpDir, "services.yaml")
	assert.Equal(t, map[string]string{servicesXML: servicesYAML}, converted)

	assert.NoFileExists(t, servicesXML)

	content, err := os.ReadFile(servicesYAML)
	require.NoError(t, err)
	assert.Equal(t, `services:
    Shop\Service:
        arguments:
            - '@logger'
`, string(content))
}

func TestConvertServicesXMLFileConvertsImports(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "packages"), 0o755))

	servicesXML := filepath.Join(tmpDir, "services.xml")
	importedXML := filepath.Join(tmpDir, "packages", "listeners.xml")

	require.NoError(t, os.WriteFile(servicesXML, []byte(`<container>
    <imports>
        <import resource="packages/listeners.xml"/>
        <import resource="packages/missing.xml"/>
    </imports>
    <services>
        <service id="Shop\Service" class="Shop\Service"/>
    </services>
</container>`), 0o644))

	require.NoError(t, os.WriteFile(importedXML, []byte(`<container>
    <services>
        <service id="Shop\Listener" class="Shop\Listener"/>
    </services>
</container>`), 0o644))

	converted, err := ConvertServicesXMLFile(servicesXML)
	require.NoError(t, err)

	assert.Equal(t, map[string]string{
		servicesXML: filepath.Join(tmpDir, "services.yaml"),
		importedXML: filepath.Join(tmpDir, "packages", "listeners.yaml"),
	}, converted)

	assert.NoFileExists(t, servicesXML)
	assert.NoFileExists(t, importedXML)

	content, err := os.ReadFile(filepath.Join(tmpDir, "services.yaml"))
	require.NoError(t, err)
	assert.Equal(t, `imports:
    - {resource: packages/listeners.yaml}
    - {resource: packages/missing.xml}

services:
    Shop\Service: ~
`, string(content))

	importedContent, err := os.ReadFile(filepath.Join(tmpDir, "packages", "listeners.yaml"))
	require.NoError(t, err)
	assert.Equal(t, `services:
    Shop\Listener: ~
`, string(importedContent))
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

func TestConvertServicesXMLFileLeavesGlobImports(t *testing.T) {
	tmpDir := t.TempDir()
	servicesXML := filepath.Join(tmpDir, "services.xml")

	require.NoError(t, os.WriteFile(servicesXML, []byte(`<container>
    <imports>
        <import resource="packages/*.xml"/>
    </imports>
</container>`), 0o644))

	converted, err := ConvertServicesXMLFile(servicesXML)
	require.NoError(t, err)
	assert.Len(t, converted, 1)

	content, err := os.ReadFile(filepath.Join(tmpDir, "services.yaml"))
	require.NoError(t, err)
	assert.Equal(t, `imports:
    - {resource: packages/*.xml}
`, string(content))
}
