package scaffolding

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every generator has to render its stubs and may only create files on the
// first run.
func TestGeneratorsCreateFilesOnceRunInAPlugin(t *testing.T) {
	for _, generator := range Generators() {
		t.Run(generator.Name, func(t *testing.T) {
			plugin := newPlugin(t)

			var args []string
			if generator.Args != "" {
				args = []string{"ExampleEntity"}
			}

			result, err := generator.Run(plugin, args)
			require.NoError(t, err)
			assert.NotEmpty(t, result.Created)

			for _, path := range result.Created {
				assert.FileExists(t, filepath.Join(plugin.Dir, filepath.FromSlash(path)))
			}

			second, err := generator.Run(plugin, args)
			require.NoError(t, err)
			assert.Empty(t, second.Created)
			assert.Empty(t, second.Updated)
		})
	}
}

func TestAdminModuleGeneratorCreatesTheExampleComponent(t *testing.T) {
	plugin := newPlugin(t)

	result, err := generatorByName(t, "admin-module").Run(plugin, nil)
	require.NoError(t, err)

	listPath := adminSrcPath + "module/swag-example/page/swag-example-list/"
	assert.Subset(t, result.Created, []string{
		adminSrcPath + "module/swag-example/index.js",
		listPath + "index.js",
		listPath + "swag-example-list.html.twig",
		listPath + "swag-example-list.scss",
	})

	// The Twig stub is copied verbatim, its {{ }} are no Go template actions.
	assert.Contains(t, readPluginFile(t, plugin, listPath+"swag-example-list.html.twig"), "{{ $t('swag-example.general.list.cardText') }}")
	assert.Contains(t, readPluginFile(t, plugin, adminSrcPath+"module/swag-example/index.js"), "import './page/swag-example-list';")
}

func TestScheduledTaskGeneratorRegistersTaskAndHandler(t *testing.T) {
	plugin := newPlugin(t)

	result, err := generatorByName(t, "scheduled-task").Run(plugin, nil)
	require.NoError(t, err)

	assert.Subset(t, result.Created, []string{
		"src/ScheduledTask/ExampleTask.php",
		"src/ScheduledTask/ExampleTaskHandler.php",
	})

	task := readPluginFile(t, plugin, "src/ScheduledTask/ExampleTask.php")
	assert.Contains(t, task, "return 'my_vendor_my_extension.example_task';")

	handler := readPluginFile(t, plugin, "src/ScheduledTask/ExampleTaskHandler.php")
	assert.Contains(t, handler, `namespace MyVendor\MyExtension\ScheduledTask;`)
	assert.Contains(t, handler, "#[AsMessageHandler(handles: ExampleTask::class)]")

	services := readPluginFile(t, plugin, servicesPath)
	assert.Contains(t, services, `$services->set(\MyVendor\MyExtension\ScheduledTask\ExampleTask::class)`)
	assert.Contains(t, services, `$services->set(\MyVendor\MyExtension\ScheduledTask\ExampleTaskHandler::class)`)
	assert.Contains(t, services, "->tag('messenger.message_handler');")
}

func TestEntityGeneratorPrefixesTableNameWithPlugin(t *testing.T) {
	plugin := newPlugin(t)

	result, err := generatorByName(t, "entity").Run(plugin, []string{"ExampleEntity"})
	require.NoError(t, err)

	assert.Contains(t, result.Created, "src/Core/Content/ExampleEntity/ExampleEntityDefinition.php")
	definition := readPluginFile(t, plugin, "src/Core/Content/ExampleEntity/ExampleEntityDefinition.php")
	assert.Contains(t, definition, "ENTITY_NAME = 'my_vendor_my_extension_example_entity'")
}

func TestEntityGeneratorRejectsInvalidEntityName(t *testing.T) {
	plugin := newPlugin(t)

	_, err := generatorByName(t, "entity").Run(plugin, []string{"notPascal"})

	assert.ErrorContains(t, err, `invalid entity name "notPascal"`)
}

func TestEntityGeneratorRejectsOversizedTableName(t *testing.T) {
	plugin := newPlugin(t)
	entity := "E" + strings.Repeat("x", 50)

	_, err := generatorByName(t, "entity").Run(plugin, []string{entity})

	assert.ErrorContains(t, err, "MySQL allows at most 64")
	assert.NoDirExists(t, filepath.Join(plugin.Dir, "src", "Core", "Content", entity))
}

func TestPluginConfigGeneratorCreatesTheConfigXML(t *testing.T) {
	plugin := newPlugin(t)

	result, err := generatorByName(t, "plugin-config").Run(plugin, nil)
	require.NoError(t, err)

	assert.Equal(t, []string{"src/Resources/config/config.xml"}, result.Created)
	assert.Contains(t, readPluginFile(t, plugin, "src/Resources/config/config.xml"), "<card>")
}

func generatorByName(t *testing.T, name string) Generator {
	t.Helper()

	for _, generator := range Generators() {
		if generator.Name == name {
			return generator
		}
	}

	t.Fatalf("unknown generator %q", name)

	return Generator{}
}

func newPlugin(t *testing.T) PluginInfo {
	t.Helper()

	return PluginInfo{
		Dir:       t.TempDir(),
		Namespace: `MyVendor\MyExtension`,
		ClassName: "MyVendorMyExtension",
	}
}

func readPluginFile(t *testing.T, plugin PluginInfo, path string) string {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(plugin.Dir, filepath.FromSlash(path)))
	require.NoError(t, err)

	return string(content)
}
