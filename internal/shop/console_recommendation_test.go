package shop

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWriteConsoleRecommendationMappings(t *testing.T) {
	cases := map[string]string{
		"plugin:create":                     "extension create --type plugin",
		"theme:create":                      "extension create --type theme",
		"make:plugin:admin-module":          "extension add admin-module",
		"make:plugin:command":               "extension add command",
		"make:plugin:config":                "extension add plugin-config",
		"make:plugin:custom-fieldset":       "extension add custom-fieldset",
		"make:plugin:entity":                "extension add entity",
		"make:plugin:event-subscriber":      "extension add event-subscriber",
		"make:plugin:javascript-plugin":     "extension add javascript-plugin",
		"make:plugin:scheduled-task":        "extension add scheduled-task",
		"make:plugin:store-api-route":       "extension add store-api-route",
		"make:plugin:storefront-controller": "extension add storefront-controller",
		"plugin:list":                       "project extension list",
		"app:list":                          "project extension list",
		"plugin:refresh":                    "project extension list",
		"plugin:install":                    "project extension install",
		"app:install":                       "project extension install",
		"plugin:activate":                   "project extension activate",
		"app:activate":                      "project extension activate",
		"plugin:deactivate":                 "project extension deactivate",
		"app:deactivate":                    "project extension deactivate",
		"plugin:uninstall":                  "project extension uninstall",
		"app:uninstall":                     "project extension uninstall",
		"plugin:update":                     "project extension update",
		"plugin:update:all":                 "project extension update all",
		"app:refresh":                       "project extension update",
		"app:update":                        "project extension update",
		"plugin:zip-import":                 "project extension upload",
		"app:validate":                      "extension validate",
		"cache:clear":                       "project clear-cache",
		"cache:clear:all":                   "project clear-cache",
		"messenger:consume":                 "project worker",
		"theme:compile":                     "project storefront-build",
		"assets:install":                    "project admin-build",
		"system:install":                    "project dev install",
		"system:update:prepare":             "project upgrade",
		"system:update:finish":              "project upgrade",
	}

	for source, target := range cases {
		t.Run(source, func(t *testing.T) {
			var out strings.Builder
			assert.Equal(t, source, WriteConsoleRecommendation(&out, []string{source}, false))
			assert.Contains(t, out.String(), `"shopware-cli `+target+`"`)
			assert.Equal(t, 1, strings.Count(out.String(), "\n"))
		})
	}

	var out strings.Builder
	WriteConsoleRecommendation(&out, []string{"assets:install"}, false)
	assert.Contains(t, out.String(), `"shopware-cli project storefront-build"`)
}

func TestWriteConsoleRecommendationSkipsUnmatchedAndSuppressedInvocations(t *testing.T) {
	cases := []struct {
		args    []string
		command string
	}{
		{nil, ""},
		{[]string{"plugin:inst", "Example"}, ""},
		{[]string{"make:plugin:composer"}, ""},
		{[]string{"app:create", "Example"}, ""},
		{[]string{"--no-interaction", "plugin:install", "Example"}, ""},
		{[]string{"help", "plugin:install"}, ""},
		{[]string{"list"}, ""},
		{[]string{"plugin:install", "--help"}, ""},
		{[]string{"plugin:install", "-h"}, ""},
		{[]string{"plugin:install", "--quiet", "--help"}, ""},
		{[]string{"plugin:install", "Example", "--quiet"}, "plugin:install"},
		{[]string{"plugin:install", "Example", "-q"}, "plugin:install"},
		{[]string{"plugin:install", "Example", "--silent"}, "plugin:install"},
		{[]string{"plugin:install", "--version"}, ""},
		{[]string{"plugin:install", "-V"}, ""},
		{[]string{"plugin:uninstall", "Example", "--keep-user-data"}, "plugin:uninstall"},
		{[]string{"app:uninstall", "Example", "--keep-user-data"}, "app:uninstall"},
	}

	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			var out strings.Builder
			assert.Equal(t, tc.command, WriteConsoleRecommendation(&out, tc.args, true))
			assert.Empty(t, out.String())
		})
	}
}

func TestWriteConsoleRecommendationPreservesProjectSelectionGuidance(t *testing.T) {
	var out strings.Builder
	assert.Equal(t, "plugin:uninstall", WriteConsoleRecommendation(&out, []string{"plugin:uninstall", "--", "--keep-user-data", "--quiet", "--help"}, true))
	assert.Contains(t, out.String(), `"shopware-cli project extension uninstall"`)
	assert.Contains(t, out.String(), "--env")
	assert.Contains(t, out.String(), "--project-config")
	assert.NotContains(t, out.String(), "--keep-user-data")

	out.Reset()
	WriteConsoleRecommendation(&out, []string{"plugin:create", "Example"}, true)
	assert.Contains(t, out.String(), `"shopware-cli extension create --type plugin"`)
	assert.NotContains(t, out.String(), "--env")
	assert.NotContains(t, out.String(), "--project-config")
}
