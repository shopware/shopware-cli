package symfony

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func convertString(t *testing.T, content string) string {
	t.Helper()

	container, err := ParseServicesXML([]byte(content))
	require.NoError(t, err)

	converted, err := ConvertContainerToYAML(container)
	require.NoError(t, err)

	// Everything we generate has to be parseable YAML.
	var parsed map[string]any
	require.NoError(t, yaml.Unmarshal(converted, &parsed))

	return string(converted)
}

func convertErr(t *testing.T, content string) error {
	t.Helper()

	container, err := ParseServicesXML([]byte(content))
	require.NoError(t, err)

	_, err = ConvertContainerToYAML(container)
	require.Error(t, err)

	return err
}

// TestConvertFixtures converts every testdata/convert/<name>/services.xml and
// compares the result with the expected.yaml next to it.
func TestConvertFixtures(t *testing.T) {
	fixturesDir := filepath.Join("testdata", "convert")

	entries, err := os.ReadDir(fixturesDir)
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		t.Run(entry.Name(), func(t *testing.T) {
			input, err := os.ReadFile(filepath.Join(fixturesDir, entry.Name(), "services.xml"))
			require.NoError(t, err)

			expected, err := os.ReadFile(filepath.Join(fixturesDir, entry.Name(), "expected.yaml"))
			require.NoError(t, err)

			assert.Equal(t, string(expected), convertString(t, string(input)))
		})
	}
}

func TestConvertErrors(t *testing.T) {
	tests := []struct {
		name        string
		xml         string
		expectedErr string
	}{
		{
			name:        "unknown element in container",
			xml:         `<container><monolog/></container>`,
			expectedErr: "unsupported element <monolog> inside <container>",
		},
		{
			name:        "stack is not supported",
			xml:         `<container><services><stack id="foo"/></services></container>`,
			expectedErr: "unsupported element <stack> inside <services>",
		},
		{
			name:        "unknown attribute on service",
			xml:         `<container><services><service id="foo" class="Foo" scope="prototype"/></services></container>`,
			expectedErr: `unsupported attribute "scope" on <service>`,
		},
		{
			name:        "unknown element in service",
			xml:         `<container><services><service id="foo" class="Foo"><something/></service></services></container>`,
			expectedErr: "unsupported element <something> inside <service>",
		},
		{
			name:        "duplicate service id",
			xml:         `<container><services><service id="foo" class="Foo"/><service id="foo" class="Bar"/></services></container>`,
			expectedErr: `service "foo" is defined multiple times`,
		},
		{
			name:        "service without id",
			xml:         `<container><services><service class="Foo"/></services></container>`,
			expectedErr: "<service> requires an id attribute",
		},
		{
			name:        "prototype without resource",
			xml:         `<container><services><prototype namespace="Foo\"/></services></container>`,
			expectedErr: "requires a resource attribute",
		},
		{
			name:        "alias with class",
			xml:         `<container><services><service id="foo" alias="bar" class="Foo"/></services></container>`,
			expectedErr: "aliases only support the public attribute",
		},
		{
			name:        "service reference without id",
			xml:         `<container><services><service id="foo" class="Foo"><argument type="service"/></service></services></container>`,
			expectedErr: `argument type="service" requires an id attribute`,
		},
		{
			name:        "unsupported argument type",
			xml:         `<container><services><service id="foo" class="Foo"><argument type="wild"/></service></services></container>`,
			expectedErr: `unsupported argument type "wild"`,
		},
		{
			name:        "unsupported on-invalid",
			xml:         `<container><services><service id="foo" class="Foo"><argument type="service" id="bar" on-invalid="explode"/></service></services></container>`,
			expectedErr: `unsupported on-invalid value "explode"`,
		},
		{
			name:        "tag without name",
			xml:         `<container><services><service id="foo" class="Foo"><tag/></service></services></container>`,
			expectedErr: "<tag> requires a name",
		},
		{
			name:        "call without method",
			xml:         `<container><services><service id="foo" class="Foo"><call/></service></services></container>`,
			expectedErr: "<call> requires a method attribute",
		},
		{
			name:        "factory without target",
			xml:         `<container><services><service id="foo" class="Foo"><factory/></service></services></container>`,
			expectedErr: "<factory> requires a class, service, function or expression attribute",
		},
		{
			name:        "inline service in factory",
			xml:         `<container><services><service id="foo" class="Foo"><factory method="create"><service class="Bar"/></factory></service></services></container>`,
			expectedErr: "unsupported element <service> inside <factory>",
		},
		{
			name:        "bind without key",
			xml:         `<container><services><service id="foo" class="Foo"><bind>value</bind></service></services></container>`,
			expectedErr: "<bind> requires a key attribute",
		},
		{
			name:        "when without env",
			xml:         `<container><when><services/></when></container>`,
			expectedErr: "<when> requires an env attribute",
		},
		{
			name:        "multiple defaults",
			xml:         `<container><services><defaults autowire="true"/><defaults public="false"/></services></container>`,
			expectedErr: "multiple <defaults> elements are not supported",
		},
		{
			name:        "namespace on regular service",
			xml:         `<container><services><service id="foo" class="Foo" namespace="Bar\"/></services></container>`,
			expectedErr: "namespace, resource and exclude are only supported on <prototype>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := convertErr(t, tt.xml)
			assert.ErrorContains(t, err, tt.expectedErr)
		})
	}
}

func TestConvertInvalidXML(t *testing.T) {
	_, err := ParseServicesXML([]byte(`<routes/>`))
	assert.Error(t, err)

	_, err = ParseServicesXML([]byte(`not xml`))
	assert.Error(t, err)
}
