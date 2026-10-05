package verifier

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/testhelper"
	"github.com/shopware/shopware-cli/internal/validation"
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

				check := NewCheck()
				check.SetSourceRoot(p.Root)
				previous := validation.CheckResult{
					Identifier: "config.services_xml.deprecated",
					Path:       "src/Resources/config/services.xml",
					Line:       1,
				}
				check.AddResult(previous)
				require.NoError(t, Builtin{}.Check(t.Context(), check, ToolConfig{
					Extensions: exts,
					RootDir:    p.Root,
				}))

				var paths []string
				for _, result := range check.GetResults() {
					if result.Identifier == previous.Identifier {
						paths = append(paths, result.Path)
					}
				}
				assert.ElementsMatch(t, []string{
					previous.Path,
					"custom/plugins/SecondPlugin/src/Resources/config/services.xml",
				}, paths)
			})
		}
	}
}
