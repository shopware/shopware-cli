package symfony

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func convertRoutesString(t *testing.T, content string) string {
	t.Helper()

	routes, err := ParseRoutesXML([]byte(content))
	require.NoError(t, err)

	converted, err := ConvertRoutesToYAML(routes)
	require.NoError(t, err)

	// Everything we generate has to be parseable YAML.
	var parsed map[string]any
	require.NoError(t, yaml.Unmarshal(converted, &parsed))

	return string(converted)
}

func convertRoutesErr(t *testing.T, content string) error {
	t.Helper()

	routes, err := ParseRoutesXML([]byte(content))
	require.NoError(t, err)

	_, err = ConvertRoutesToYAML(routes)
	require.Error(t, err)

	return err
}

// TestConvertRoutesFixtures converts every testdata/routes/<name>/routes.xml
// and compares the result with the expected.yaml next to it.
func TestConvertRoutesFixtures(t *testing.T) {
	fixturesDir := filepath.Join("testdata", "routes")

	entries, err := os.ReadDir(fixturesDir)
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		t.Run(entry.Name(), func(t *testing.T) {
			input, err := os.ReadFile(filepath.Join(fixturesDir, entry.Name(), "routes.xml"))
			require.NoError(t, err)

			expected, err := os.ReadFile(filepath.Join(fixturesDir, entry.Name(), "expected.yaml"))
			require.NoError(t, err)

			assert.Equal(t, string(expected), convertRoutesString(t, string(input)))
		})
	}
}

func TestConvertRoutesErrors(t *testing.T) {
	tests := []struct {
		name        string
		xml         string
		expectedErr string
	}{
		{
			name:        "route without id",
			xml:         `<routes><route path="/foo"/></routes>`,
			expectedErr: "<route> requires an id attribute",
		},
		{
			name:        "route without path",
			xml:         `<routes><route id="foo"/></routes>`,
			expectedErr: "requires a path attribute or <path> child elements",
		},
		{
			name:        "path attribute and child elements",
			xml:         `<routes><route id="foo" path="/foo"><path locale="en">/en/foo</path></route></routes>`,
			expectedErr: "must not have both a path attribute and <path> child elements",
		},
		{
			name:        "controller attribute and _controller default",
			xml:         `<routes><route id="foo" path="/foo" controller="Foo"><default key="_controller">Bar</default></route></routes>`,
			expectedErr: "must not specify both the controller attribute and the _controller default",
		},
		{
			name:        "duplicate route id",
			xml:         `<routes><route id="foo" path="/foo"/><route id="foo" path="/bar"/></routes>`,
			expectedErr: `route "foo" is defined multiple times`,
		},
		{
			name:        "unknown element",
			xml:         `<routes><something/></routes>`,
			expectedErr: "unsupported element <something> inside <routes>",
		},
		{
			name:        "unknown attribute on route",
			xml:         `<routes><route id="foo" path="/foo" bogus="1"/></routes>`,
			expectedErr: `unsupported attribute "bogus" on <route>`,
		},
		{
			name:        "unknown element in route",
			xml:         `<routes><route id="foo" path="/foo"><something/></route></routes>`,
			expectedErr: "unsupported element <something> inside <route>",
		},
		{
			name:        "import without resource",
			xml:         `<routes><import type="attribute"/></routes>`,
			expectedErr: "<import> requires a resource attribute",
		},
		{
			name:        "exclude attribute mixed with exclude elements",
			xml:         `<routes><import resource="foo" exclude="bar"><exclude>baz</exclude></import></routes>`,
			expectedErr: "mixes the exclude attribute with <exclude> elements",
		},
		{
			name:        "prefix attribute and child elements",
			xml:         `<routes><import resource="foo" prefix="/en"><prefix locale="en">/en</prefix></import></routes>`,
			expectedErr: "must not have both a prefix attribute and <prefix> child elements",
		},
		{
			name:        "host attribute and child elements",
			xml:         `<routes><route id="foo" path="/foo" host="example.com"><host locale="en">en.example.com</host></route></routes>`,
			expectedErr: "must not have both a host attribute and <host> child elements",
		},
		{
			name:        "when without env",
			xml:         `<routes><when><route id="foo" path="/foo"/></when></routes>`,
			expectedErr: "<when> requires an env attribute",
		},
		{
			name:        "nested when",
			xml:         `<routes><when env="dev"><when env="test"><route id="foo" path="/foo"/></when></when></routes>`,
			expectedErr: "<when> elements cannot be nested",
		},
		{
			name:        "duplicate when env",
			xml:         `<routes><when env="dev"><route id="a" path="/a"/></when><when env="dev"><route id="b" path="/b"/></when></routes>`,
			expectedErr: `environment "dev" is configured multiple times`,
		},
		{
			name:        "requirement without key",
			xml:         `<routes><route id="foo" path="/foo"><requirement>\d+</requirement></route></routes>`,
			expectedErr: "<requirement> requires a key attribute",
		},
		{
			name:        "option without key",
			xml:         `<routes><route id="foo" path="/foo"><option>true</option></route></routes>`,
			expectedErr: "<option> requires a key attribute",
		},
		{
			name:        "default without key",
			xml:         `<routes><route id="foo" path="/foo"><default>1</default></route></routes>`,
			expectedErr: "<default> requires a key attribute",
		},
		{
			name:        "multiple typed values in one default",
			xml:         `<routes><route id="foo" path="/foo"><default key="x"><bool>true</bool><int>1</int></default></route></routes>`,
			expectedErr: "only one typed value element is allowed",
		},
		{
			name:        "unsupported typed value element",
			xml:         `<routes><route id="foo" path="/foo"><default key="x"><money>5</money></default></route></routes>`,
			expectedErr: "unsupported typed value element <money>",
		},
		{
			name:        "invalid int value",
			xml:         `<routes><route id="foo" path="/foo"><default key="x"><int>abc</int></default></route></routes>`,
			expectedErr: `invalid <int> value "abc"`,
		},
		{
			name:        "map entry without key",
			xml:         `<routes><route id="foo" path="/foo"><default key="x"><map><string>v</string></map></default></route></routes>`,
			expectedErr: "<map> entries require a key attribute",
		},
		{
			name:        "localized path without locale",
			xml:         `<routes><route id="foo"><path>/foo</path></route></routes>`,
			expectedErr: "<path> child elements require a locale attribute",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := convertRoutesErr(t, tt.xml)
			assert.ErrorContains(t, err, tt.expectedErr)
		})
	}
}

func TestConvertRoutesInvalidXML(t *testing.T) {
	_, err := ParseRoutesXML([]byte(`<container/>`))
	assert.Error(t, err)

	_, err = ParseRoutesXML([]byte(`not xml`))
	assert.Error(t, err)
}

func TestRouteImportKey(t *testing.T) {
	tests := []struct {
		name     string
		resource string
		used     []string
		want     string
	}{
		{name: "controller glob", resource: "../../Storefront/Controller/**/*Controller.php", want: "storefront_controller"},
		{name: "directory", resource: "../src/Controller", want: "src_controller"},
		{name: "file extension is stripped", resource: "routes/admin.xml", want: "routes_admin"},
		{name: "glob only", resource: "*.php", want: "imported_routes"},
		{name: "collision gets a suffix", resource: "routes/admin.xml", used: []string{"routes_admin"}, want: "routes_admin_2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			used := map[string]bool{}
			for _, key := range tt.used {
				used[key] = true
			}

			assert.Equal(t, tt.want, routeImportKey(tt.resource, used))
		})
	}
}
