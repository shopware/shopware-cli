package proxy

import "path/filepath"

// CanonicalProjectRoot resolves symlinks in projectRoot so the registry is
// keyed consistently regardless of how a project directory was reached. If
// symlinks cannot be resolved, projectRoot is returned unchanged.
func CanonicalProjectRoot(projectRoot string) string {
	resolved, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		return projectRoot
	}

	return resolved
}
