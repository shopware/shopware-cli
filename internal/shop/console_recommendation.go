package shop

import (
	"fmt"
	"io"
	"strings"
)

type consoleRecommendation struct {
	command string
	purpose string
	detail  string
}

const (
	generatorDetail = "Run it from the plugin directory on Shopware 6.7.13.0 or newer."
	lifecycleDetail = "Requires Admin API access and runtime extension management on the target shop."
	installDetail   = "Pass the extension name and add --activate to activate it on installation. " + lifecycleDetail
	uninstall       = "project extension uninstall"
	appUpdate       = "update an active app listed by the target shop"
	appUpdateDetail = "Pass the app name. Use app:refresh to sync apps with local manifests. " + lifecycleDetail
	localCacheOnly  = "Without Admin API credentials, it only removes the local var/cache directory."
	upgradePurpose  = "run a guided upgrade of your local project"
	upgradeDetail   = "The workflow checks compatibility, updates dependencies, and writes an upgrade report."
)

var consoleRecommendations = map[string]consoleRecommendation{
	"plugin:create":                     {"extension create --type plugin", "scaffold a plugin in your project", `Pass --name to set its name. Add features from the plugin directory with "shopware-cli extension add".`},
	"theme:create":                      {"extension create --type theme", "scaffold a Storefront theme in your project", "Pass --name to set its name."},
	"make:plugin:admin-module":          {"extension add admin-module", "scaffold an Administration module", generatorDetail},
	"make:plugin:command":               {"extension add command", "scaffold a console command", generatorDetail},
	"make:plugin:config":                {"extension add plugin-config", "scaffold plugin configuration", generatorDetail},
	"make:plugin:custom-fieldset":       {"extension add custom-fieldset", "scaffold a custom field set", generatorDetail},
	"make:plugin:entity":                {"extension add entity", "scaffold an entity in your plugin", generatorDetail + " Pass the entity name."},
	"make:plugin:event-subscriber":      {"extension add event-subscriber", "scaffold an event subscriber", generatorDetail},
	"make:plugin:javascript-plugin":     {"extension add javascript-plugin", "scaffold a Storefront JavaScript plugin", generatorDetail},
	"make:plugin:scheduled-task":        {"extension add scheduled-task", "scaffold a scheduled task", generatorDetail},
	"make:plugin:store-api-route":       {"extension add store-api-route", "scaffold a Store API route", generatorDetail},
	"make:plugin:storefront-controller": {"extension add storefront-controller", "scaffold a Storefront controller", generatorDetail},
	"plugin:list":                       {"project extension list", "see plugin versions and status via the Admin API", ""},
	"app:list":                          {"project extension list", "see managed apps and plugins together via the Admin API", "Use app:list to include self-managed apps."},
	"plugin:refresh":                    {"project extension list", "refresh plugin discovery and see extension status in one step", lifecycleDetail},
	"plugin:install":                    {"project extension install", "install a plugin that the target shop lists as uninstalled", installDetail},
	"app:install":                       {"project extension install", "install an app available in the target shop's extension list", installDetail + " Use app:install for an app that's only present as a local manifest."},
	"plugin:activate":                   {"project extension activate", "activate a plugin by name", lifecycleDetail},
	"app:activate":                      {"project extension activate", "activate an app listed by the target shop", lifecycleDetail},
	"plugin:deactivate":                 {"project extension deactivate", "deactivate a plugin by name", lifecycleDetail},
	"app:deactivate":                    {"project extension deactivate", "deactivate an app listed by the target shop", lifecycleDetail},
	"plugin:uninstall":                  {uninstall, "uninstall a plugin by name", lifecycleDetail},
	"app:uninstall":                     {uninstall, "uninstall an app listed by the target shop", lifecycleDetail},
	"plugin:update":                     {"project extension update", "update active plugins by name", "It can download available Store updates. " + lifecycleDetail},
	"plugin:update:all":                 {"project extension update all", "update active plugins and apps together", "It can download available Store updates. " + lifecycleDetail},
	"app:refresh":                       {"project extension update", appUpdate, appUpdateDetail},
	"app:update":                        {"project extension update", appUpdate, appUpdateDetail},
	"plugin:zip-import":                 {"project extension upload", "upload a plugin ZIP to the target shop", "Pass the local ZIP path. " + lifecycleDetail},
	"app:validate":                      {"extension validate", "check an app's metadata, assets, and code quality", "Pass the app folder or ZIP path."},
	"cache:clear":                       {"project clear-cache", "clear project caches", "With Admin API credentials, it clears the target shop's caches. " + localCacheOnly},
	"cache:clear:all":                   {"project clear-cache", "clear the target shop's cache pools through the Admin API", localCacheOnly},
	"messenger:consume":                 {"project worker", "run supervised Messenger consumers", "Select queues with --queue and let the CLI restart consumers when they exit."},
	"theme:compile":                     {"project storefront-build", "build and install Storefront assets and compile themes in one step", ""},
	"assets:install":                    {"project admin-build", "build and install Administration assets together", `To build and install Storefront assets, use "shopware-cli project storefront-build".`},
	"system:install":                    {"project dev install", "set up a development shop without prompts", "It starts the environment and saves Admin API credentials after a new installation."},
	"system:update:prepare":             {"project upgrade", upgradePurpose, upgradeDetail},
	"system:update:finish":              {"project upgrade", upgradePurpose, upgradeDetail},
}

// WriteConsoleRecommendation writes optional guidance and returns the allowlisted
// Symfony command name for telemetry. Help and version requests return no name.
func WriteConsoleRecommendation(w io.Writer, args []string, explicitProjectSelection bool) string {
	if len(args) == 0 {
		return ""
	}

	name := args[0]
	recommendation, ok := consoleRecommendations[name]
	if !ok {
		return ""
	}

	showHint := true
	for _, arg := range args[1:] {
		if arg == "--" {
			break
		}

		option, _, _ := strings.Cut(arg, "=")
		switch option {
		case "--help", "--version":
			return ""
		case "--quiet", "--silent":
			showHint = false
		case "--keep-user-data":
			if recommendation.command == uninstall {
				showHint = false
			}
		}
		if strings.HasPrefix(option, "-h") || strings.HasPrefix(option, "-V") {
			return ""
		}
		if strings.HasPrefix(option, "-q") {
			showHint = false
		}
	}
	if !showHint {
		return name
	}

	detail := recommendation.detail
	if detail != "" {
		detail = " " + detail
	}
	if explicitProjectSelection && strings.HasPrefix(recommendation.command, "project ") {
		detail += " Reuse any explicit --env or --project-config selection."
	}
	// The hint is advisory. A write failure must not prevent the console command from running.
	_, _ = fmt.Fprintf(w, "To %s, use \"shopware-cli %s\".%s\n", recommendation.purpose, recommendation.command, detail)

	return name
}
