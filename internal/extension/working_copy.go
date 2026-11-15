package extension

import (
	"fmt"
	"os"
	"path/filepath"

	cp "github.com/otiai10/copy"

	"github.com/shopware/shopware-cli/internal/git"
)

// CopyWorkingCopy copies the extension folder as it is on disk, without asking Git. Symlinks are skipped.
// From CompatibilityExportIgnore on, paths marked export-ignore in .gitattributes are left out,
// the same way "git archive" leaves them out of a Git based package.
func CopyWorkingCopy(source, target string, cfg *Config) error {
	options := cp.Options{
		OnSymlink: func(string) cp.SymlinkAction {
			return cp.Skip
		},
	}

	if !cfg.IsCompatibilityDateBefore(CompatibilityExportIgnore) {
		exportIgnore := git.NewExportIgnore(source)

		options.Skip = func(info os.FileInfo, src, _ string) (bool, error) {
			relPath, err := filepath.Rel(source, src)
			if err != nil {
				return false, fmt.Errorf("cannot resolve %s inside %s: %w", src, source, err)
			}

			return exportIgnore.Match(relPath, info.IsDir())
		}
	}

	if err := cp.Copy(source, target, options); err != nil {
		return fmt.Errorf("cannot copy %s: %w", source, err)
	}

	return nil
}
