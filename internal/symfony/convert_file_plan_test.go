package symfony

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanXMLConversionMatchesAppliedFiles(t *testing.T) {
	for _, scenario := range []struct {
		name, source, child string
		plan, convert       func(string) (map[string]string, error)
	}{
		{
			name: "services", source: `<container><imports><import resource="child.xml"/></imports></container>`,
			child: `<container/>`, plan: PlanServicesXMLFile, convert: ConvertServicesXMLFile,
		},
		{
			name: "routes", source: `<routes><import resource="child.xml"/></routes>`,
			child: `<routes/>`, plan: PlanRoutesXMLFile, convert: ConvertRoutesXMLFile,
		},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, scenario.name+".xml")
			child := filepath.Join(root, "child.xml")
			require.NoError(t, os.WriteFile(source, []byte(scenario.source), 0o600))
			require.NoError(t, os.WriteFile(child, []byte(scenario.child), 0o600))

			planned, err := scenario.plan(source)
			require.NoError(t, err)
			assert.Equal(t, map[string]string{
				source: filepath.Join(root, scenario.name+".yaml"),
				child:  filepath.Join(root, "child.yaml"),
			}, planned)
			for file, original := range map[string]string{source: scenario.source, child: scenario.child} {
				contents, err := os.ReadFile(file)
				require.NoError(t, err)
				assert.Equal(t, original, string(contents))
				info, err := os.Stat(file)
				require.NoError(t, err)
				assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
			}
			for _, target := range planned {
				assert.NoFileExists(t, target)
			}

			converted, err := scenario.convert(source)
			require.NoError(t, err)
			assert.Equal(t, planned, converted)
			for file, target := range converted {
				assert.NoFileExists(t, file)
				assert.FileExists(t, target)
			}
		})
	}
}

func TestPlanXMLConversionRetainsConflictingTarget(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "services.xml")
	target := filepath.Join(root, "services.yaml")
	require.NoError(t, os.WriteFile(source, []byte(`<container/>`), 0o600))
	require.NoError(t, os.WriteFile(target, []byte("existing content"), 0o600))

	planned, err := PlanServicesXMLFile(source)
	var conversionErr *XMLConversionError
	require.ErrorAs(t, err, &conversionErr)
	assert.Equal(t, XMLConversionTargetExists, conversionErr.Kind)
	assert.Equal(t, source, conversionErr.Path)
	assert.Empty(t, planned)
	contents, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "existing content", string(contents))
	assert.FileExists(t, source)
}
