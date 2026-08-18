package extension

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testPrepareAppManifest = `<?xml version="1.0" encoding="UTF-8"?>
<manifest xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:noNamespaceSchemaLocation="https://raw.githubusercontent.com/shopware/shopware/trunk/src/Core/Framework/App/Manifest/Schema/manifest-2.0.xsd">
	<meta>
		<name>MyExampleApp</name>
		<label>Label</label>
		<description>A description</description>
		<author>Your Company Ltd.</author>
		<copyright>(c) by Your Company Ltd.</copyright>
		<version>1.0.0</version>
		<license>MIT</license>
	</meta>
</manifest>`

func TestExtensionPrepareCommandDeprecated(t *testing.T) {
	t.Parallel()

	assert.Contains(t, extensionPrepareCmd.Deprecated, "October 2026")
	assert.Contains(t, extensionPrepareCmd.Deprecated, "extension package")
}

func TestExtensionPreparePrintsDeprecationNotice(t *testing.T) {
	out := new(bytes.Buffer)
	extensionPrepareCmd.SetOut(out)
	extensionPrepareCmd.SetErr(out)
	extensionPrepareCmd.SetArgs([]string{"--help"})
	t.Cleanup(func() {
		extensionPrepareCmd.SetArgs(nil)
		extensionPrepareCmd.SetOut(nil)
		extensionPrepareCmd.SetErr(nil)
	})

	require.NoError(t, extensionPrepareCmd.Execute())

	output := out.String()
	assert.Contains(t, output, "deprecated")
	assert.Contains(t, output, "October 2026")
}

func TestExtensionPrepareStillExecutes(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.xml"), []byte(testPrepareAppManifest), 0o644))

	out := new(bytes.Buffer)
	extensionPrepareCmd.SetOut(out)
	extensionPrepareCmd.SetErr(out)
	extensionPrepareCmd.SetArgs([]string{dir})
	extensionPrepareCmd.SetContext(t.Context())
	t.Cleanup(func() {
		extensionPrepareCmd.SetArgs(nil)
		extensionPrepareCmd.SetOut(nil)
		extensionPrepareCmd.SetErr(nil)
	})

	require.NoError(t, extensionPrepareCmd.Execute())
	assert.Contains(t, out.String(), "deprecated")
	assert.Contains(t, out.String(), "October 2026")
}
