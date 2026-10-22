package verifier

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestBuiltinScopesExtensionIgnores(t *testing.T) {
	for _, ignorePath := range []string{"", "src/Resources/config/services.xml"} {
		for _, ignoredFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("path=%s/ignored-first=%t", ignorePath, ignoredFirst), func(t *testing.T) {
				p := testhelper.NewProject(t)
				var exts []extension.Extension
				for _, name := range []string{"FirstPlugin", "SecondPlugin"} {
					p.CustomPlugin(name, testhelper.PluginComposer("test/"+name, "1.0.0", name+`\`+name))
					pluginDir := filepath.Join(p.Root, "custom", "plugins", name)
					writeDeprecatedServicesXML(t, pluginDir)
					ext, err := extension.GetExtensionByFolder(t.Context(), pluginDir)
					require.NoError(t, err)
					exts = append(exts, ext)
				}
				exts[0].GetExtensionConfig().Validation.Ignore = extension.ConfigValidationList{{
					Identifier: "config.services_xml.deprecated",
					Path:       ignorePath,
				}}
				if !ignoredFirst {
					exts[0], exts[1] = exts[1], exts[0]
				}

				toolResult := runCheckTool(t, Builtin{}, ToolConfig{
					Extensions: exts,
					RootDir:    p.Root,
				})
				require.Empty(t, toolResult.Errors)

				var paths []string
				for _, result := range toolResult.Findings {
					if result.Identifier == "config.services_xml.deprecated" {
						paths = append(paths, result.Path)
					}
				}
				assert.ElementsMatch(t, []string{
					"custom/plugins/SecondPlugin/src/Resources/config/services.xml",
				}, paths)
			})
		}
	}
}
