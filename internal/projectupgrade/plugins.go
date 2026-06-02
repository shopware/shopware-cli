package projectupgrade

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shopware/shopware-cli/internal/packagist"
)

// FindNonComposerPlugins returns directories under custom/plugins/ that are
// not tracked by composer (no entry in vendor/composer/installed.json).
// Returns an empty slice when no installed.json is present.
func FindNonComposerPlugins(projectRoot string) ([]string, error) {
	customPlugins := filepath.Join(projectRoot, "custom", "plugins")
	entries, err := os.ReadDir(customPlugins)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", customPlugins, err)
	}

	composerTracked := make(map[string]struct{})
	// Best-effort: a missing or malformed installed.json simply means nothing
	// is tracked, so every plugin directory is reported.
	installed, _ := packagist.ReadInstalledJson(projectRoot)
	if installed != nil {
		for _, pkg := range installed.Packages {
			if dir, ok := pkg.InstallDirName(projectRoot, customPlugins); ok {
				composerTracked[dir] = struct{}{}
			}
		}
	}

	orphans := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if _, tracked := composerTracked[entry.Name()]; tracked {
			continue
		}
		orphans = append(orphans, entry.Name())
	}

	sort.Strings(orphans)
	return orphans, nil
}
