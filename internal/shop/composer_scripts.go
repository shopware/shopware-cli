package shop

import (
	"cmp"
	"path/filepath"
	"slices"

	"github.com/shyim/go-composer"
)

// composerEventScripts are Composer hooks and Symfony Flex internals, not user commands.
var composerEventScripts = map[string]struct{}{
	"auto-scripts":              {},
	"pre-archive-cmd":           {},
	"pre-autoload-dump":         {},
	"pre-command-run":           {},
	"pre-file-download":         {},
	"pre-install-cmd":           {},
	"pre-operations-exec":       {},
	"pre-package-install":       {},
	"pre-package-uninstall":     {},
	"pre-package-update":        {},
	"pre-pool-create":           {},
	"pre-status-cmd":            {},
	"pre-update-cmd":            {},
	"post-archive-cmd":          {},
	"post-autoload-dump":        {},
	"post-create-project-cmd":   {},
	"post-file-download":        {},
	"post-install-cmd":          {},
	"post-package-install":      {},
	"post-package-uninstall":    {},
	"post-package-update":       {},
	"post-root-package-install": {},
	"post-status-cmd":           {},
	"post-update-cmd":           {},
}

type ComposerScript struct {
	Name        string
	Description string
	Aliases     []string
}

func GetComposerScripts(projectRoot string) ([]ComposerScript, error) {
	composerJSON, err := composer.ReadJson(filepath.Join(projectRoot, "composer.json"))
	if err != nil {
		return nil, err
	}

	scripts := make([]ComposerScript, 0, len(composerJSON.Scripts))
	for name := range composerJSON.Scripts {
		if _, isEvent := composerEventScripts[name]; isEvent {
			continue
		}

		script := ComposerScript{Name: name}
		if composerJSON.ScriptsDescriptions != nil {
			script.Description = composerJSON.ScriptsDescriptions[name]
		}
		if composerJSON.ScriptsAliases != nil {
			script.Aliases = slices.Clone(composerJSON.ScriptsAliases[name])
		}

		scripts = append(scripts, script)
	}

	slices.SortFunc(scripts, func(a, b ComposerScript) int {
		return cmp.Compare(a.Name, b.Name)
	})

	return scripts, nil
}

func FindComposerScript(scripts []ComposerScript, name string) (ComposerScript, bool) {
	for _, script := range scripts {
		if script.Name == name {
			return script, true
		}
	}

	for _, script := range scripts {
		if slices.Contains(script.Aliases, name) {
			return script, true
		}
	}

	return ComposerScript{}, false
}
