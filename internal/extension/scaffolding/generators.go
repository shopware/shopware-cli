package scaffolding

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	servicesPath   = "src/Resources/config/services.php"
	routesPath     = "src/Resources/config/routes.php"
	adminSrcPath   = "src/Resources/app/administration/src/"
	storefrontPath = "src/Resources/app/storefront/src/"
	viewsPath      = "src/Resources/views/storefront/"
)

// Shopware loads services and routes of a plugin from these two files. Both are
// a PHP closure, so a generator adds its block in front of the closing "};".
const (
	servicesIntro = `<?php declare(strict_types=1);

use Symfony\Component\DependencyInjection\Loader\Configurator\ContainerConfigurator;

use function Symfony\Component\DependencyInjection\Loader\Configurator\service;

return static function (ContainerConfigurator $containerConfigurator): void {
    $services = $containerConfigurator->services();
`

	routesIntro = `<?php declare(strict_types=1);

use Symfony\Component\Routing\Loader\Configurator\RoutingConfigurator;

return static function (RoutingConfigurator $routes): void {
`

	configOutro = "};\n"
)

var entityNameRegexp = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*$`)

// Generators returns every generator that can be run inside an existing plugin.
func Generators() []Generator {
	return []Generator{
		{
			Name:  "admin-module",
			Short: "Create an example administration module",
			build: buildAdminModule,
		},
		{
			Name:  "command",
			Short: "Create an example console command",
			build: buildCommand,
		},
		{
			Name:  "custom-fieldset",
			Short: "Create an example custom field set",
			build: buildCustomFieldset,
		},
		{
			Name:  "entity",
			Short: "Create an entity with definition, collection and migration",
			Args:  "ENTITY...",
			build: buildEntity,
		},
		{
			Name:  "event-subscriber",
			Short: "Create an example event subscriber",
			build: buildEventSubscriber,
		},
		{
			Name:  "javascript-plugin",
			Short: "Create an example storefront JavaScript plugin",
			build: buildJavascriptPlugin,
		},
		{
			Name:  "plugin-config",
			Short: "Create an example plugin configuration",
			build: buildPluginConfig,
		},
		{
			Name:  "scheduled-task",
			Short: "Create an example scheduled task",
			build: buildScheduledTask,
		},
		{
			Name:  "store-api-route",
			Short: "Create an example store-api route",
			build: buildStoreAPIRoute,
		},
		{
			Name:  "storefront-controller",
			Short: "Create an example storefront controller",
			build: buildStorefrontController,
		},
	}
}

func buildAdminModule(_ PluginInfo, _ []string) (output, error) {
	const moduleImport = `// Import admin module
import './module/swag-example';
`

	const listPath = adminSrcPath + "module/swag-example/page/swag-example-list/"

	return output{
		Files: []file{
			{Path: adminSrcPath + "module/swag-example/index.js", Stub: "stubs/make/admin_module.js", Raw: true},
			{Path: listPath + "index.js", Stub: "stubs/make/admin_component_index.js", Raw: true},
			{Path: listPath + "swag-example-list.html.twig", Stub: "stubs/make/admin_component_template.html.twig", Raw: true},
			{Path: listPath + "swag-example-list.scss", Stub: "stubs/make/admin_component_styling.scss", Raw: true},
			{Path: adminSrcPath + "snippet/en.json", Stub: "stubs/make/admin_snippet.json", Raw: true},
			{Path: adminSrcPath + "snippet/de.json", Stub: "stubs/make/admin_snippet.json", Raw: true},
		},
		// Shopware creates main.js instead of appending to it, which drops the
		// import as soon as the plugin already has an entry point. Appending
		// keeps the module reachable without touching the existing code.
		Snippets: []snippet{{Path: adminSrcPath + "main.js", Content: moduleImport}},
	}, nil
}

func buildCommand(plugin PluginInfo, _ []string) (output, error) {
	const service = `
    $services->set(\%s\Command\ExampleCommand::class)
        ->tag('console.command');
`

	return output{
		Files: []file{
			{Path: "src/Command/ExampleCommand.php", Stub: "stubs/make/command.php.tmpl", Data: plugin.data()},
		},
		Snippets: []snippet{servicesSnippet(fmt.Sprintf(service, plugin.Namespace))},
	}, nil
}

func buildCustomFieldset(_ PluginInfo, _ []string) (output, error) {
	return output{
		Files: []file{
			{Path: "src/Resources/config/custom-fields.xml", Stub: "stubs/make/custom_fields.xml", Raw: true},
		},
	}, nil
}

func buildEventSubscriber(plugin PluginInfo, _ []string) (output, error) {
	const service = `
    $services->set(\%s\Subscriber\MySubscriber::class)
        ->tag('kernel.event_subscriber');
`

	return output{
		Files: []file{
			{Path: "src/Subscriber/MySubscriber.php", Stub: "stubs/make/event_subscriber.php.tmpl", Data: plugin.data()},
		},
		Snippets: []snippet{servicesSnippet(fmt.Sprintf(service, plugin.Namespace))},
	}, nil
}

func buildJavascriptPlugin(_ PluginInfo, _ []string) (output, error) {
	const pluginRegistration = `// Import all necessary Storefront plugins
import ExamplePlugin from './example-plugin/example-plugin.plugin';

// Register your plugin via the existing PluginManager
const PluginManager = window.PluginManager;

PluginManager.register('ExamplePlugin', ExamplePlugin, '[data-example-plugin]');
`

	return output{
		Files: []file{
			{Path: storefrontPath + "example-plugin/example-plugin.plugin.js", Stub: "stubs/make/javascript_plugin.js", Raw: true},
			{Path: viewsPath + "page/content/index.html.twig", Stub: "stubs/make/javascript_plugin_template.html.twig", Raw: true},
		},
		Snippets: []snippet{{Path: storefrontPath + "main.js", Content: pluginRegistration}},
	}, nil
}

func buildPluginConfig(_ PluginInfo, _ []string) (output, error) {
	return output{
		Files: []file{
			{Path: "src/Resources/config/config.xml", Stub: "stubs/make/plugin_config.xml", Raw: true},
		},
	}, nil
}

func buildScheduledTask(plugin PluginInfo, _ []string) (output, error) {
	const service = `
    $services->set(\%[1]s\ScheduledTask\ExampleTask::class)
        ->tag('shopware.scheduled.task');
    $services->set(\%[1]s\ScheduledTask\ExampleTaskHandler::class)
        ->args([
            service('scheduled_task.repository'),
            service('logger'),
        ])
        ->tag('messenger.message_handler');
`

	return output{
		Files: []file{
			{Path: "src/ScheduledTask/ExampleTask.php", Stub: "stubs/make/scheduled_task.php.tmpl", Data: plugin.data()},
			{Path: "src/ScheduledTask/ExampleTaskHandler.php", Stub: "stubs/make/scheduled_task_handler.php.tmpl", Data: plugin.data()},
		},
		Snippets: []snippet{servicesSnippet(fmt.Sprintf(service, plugin.Namespace))},
	}, nil
}

func buildStoreAPIRoute(plugin PluginInfo, _ []string) (output, error) {
	const service = `
    $services->set(\%s\Core\Content\Example\SalesChannel\ExampleRoute::class)
        ->public()
        ->args([
            service('product.repository'),
        ]);
`

	const route = `
    $routes->import('../../Core/**/*Route.php', 'attribute');
`

	const salesChannelPath = "src/Core/Content/Example/SalesChannel/"

	return output{
		Files: []file{
			{Path: salesChannelPath + "AbstractExampleRoute.php", Stub: "stubs/make/store_api_abstract_route.php.tmpl", Data: plugin.data()},
			{Path: salesChannelPath + "ExampleRoute.php", Stub: "stubs/make/store_api_route.php.tmpl", Data: plugin.data()},
			{Path: salesChannelPath + "ExampleRouteResponse.php", Stub: "stubs/make/store_api_response.php.tmpl", Data: plugin.data()},
		},
		Snippets: []snippet{
			servicesSnippet(fmt.Sprintf(service, plugin.Namespace)),
			routesSnippet(route),
		},
	}, nil
}

func buildStorefrontController(plugin PluginInfo, _ []string) (output, error) {
	const service = `
    $services->set(\%s\Storefront\Controller\ExampleController::class)
        ->public()
        ->call('setContainer', [service('service_container')]);
`

	const route = `
    $routes->import('../../Storefront/Controller/**/*Controller.php', 'attribute');
`

	return output{
		Files: []file{
			{Path: "src/Storefront/Controller/ExampleController.php", Stub: "stubs/make/storefront_controller.php.tmpl", Data: plugin.data()},
			{Path: viewsPath + "page/example.html.twig", Stub: "stubs/make/storefront_template.html.twig", Raw: true},
		},
		Snippets: []snippet{
			servicesSnippet(fmt.Sprintf(service, plugin.Namespace)),
			routesSnippet(route),
		},
	}, nil
}

func buildEntity(plugin PluginInfo, entities []string) (output, error) {
	const service = `
    $services->set(\%s\Core\Content\%s\%sDefinition::class)
        ->tag('shopware.entity.definition', ['entity' => '%s']);
`

	// All migrations of one run share a timestamp; the entity name keeps the
	// class names unique.
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	var out output

	for _, entity := range entities {
		if !entityNameRegexp.MatchString(entity) {
			return output{}, fmt.Errorf("invalid entity name %q: use PascalCase, e.g. ExampleEntity", entity)
		}

		data := templateData{
			Namespace:  plugin.Namespace,
			ClassName:  plugin.ClassName,
			EntityName: entity,
			TableName:  tableName(entity),
			Timestamp:  timestamp,
		}

		contentPath := "src/Core/Content/" + entity + "/"

		migration, err := migrationFile(plugin.Dir, data)
		if err != nil {
			return output{}, err
		}

		out.Files = append(out.Files,
			file{Path: contentPath + entity + "Entity.php", Stub: "stubs/make/entity.php.tmpl", Data: data},
			file{Path: contentPath + entity + "Definition.php", Stub: "stubs/make/entity_definition.php.tmpl", Data: data},
			file{Path: contentPath + entity + "Collection.php", Stub: "stubs/make/entity_collection.php.tmpl", Data: data},
			migration,
		)

		out.Snippets = append(out.Snippets, servicesSnippet(
			fmt.Sprintf(service, plugin.Namespace, entity, entity, data.TableName),
		))
	}

	return out, nil
}

// migrationFile returns the migration that creates the entity table. The file
// name carries the creation timestamp, so an earlier migration for the same
// entity can only be found by a glob. When one exists, its path is reused and
// the migration is reported as skipped instead of being written a second time.
func migrationFile(pluginDir string, data templateData) (file, error) {
	f := file{
		Path: fmt.Sprintf("src/Migration/Migration%sCreate%sTable.php", data.Timestamp, data.EntityName),
		Stub: "stubs/make/entity_migration.php.tmpl",
		Data: data,
	}

	pattern := filepath.Join(pluginDir, "src", "Migration", "Migration*Create"+data.EntityName+"Table.php")

	existing, err := filepath.Glob(pattern)
	if err != nil {
		return file{}, fmt.Errorf("look for existing migrations: %w", err)
	}

	if len(existing) > 0 {
		f.Path = "src/Migration/" + filepath.Base(existing[0])
	}

	return f, nil
}

func (p PluginInfo) data() templateData {
	return templateData{Namespace: p.Namespace, ClassName: p.ClassName}
}

func servicesSnippet(content string) snippet {
	return snippet{Path: servicesPath, Content: content, Intro: servicesIntro, Outro: configOutro}
}

func routesSnippet(content string) snippet {
	return snippet{Path: routesPath, Content: content, Intro: routesIntro, Outro: configOutro}
}

// tableName turns a PascalCase entity name into its snake_case table name.
func tableName(entityName string) string {
	return strings.ToLower(strings.Join(splitPascalCase(entityName), "_"))
}
