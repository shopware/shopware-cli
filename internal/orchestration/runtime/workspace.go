package runtime

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// Workspace describes one source root and its corresponding POSIX target root.
// Paths are mapped lexically. The caller owns symlink resolution and mount setup.
type Workspace struct {
	sourceRoot string
	targetRoot string
}

// NewWorkspace creates a mapping from absolute source and POSIX target roots.
func NewWorkspace(sourceRoot, targetRoot string) (*Workspace, error) {
	if !filepath.IsAbs(sourceRoot) {
		return nil, fmt.Errorf("source root must be absolute: %q", sourceRoot)
	}
	if !path.IsAbs(targetRoot) {
		return nil, fmt.Errorf("target root must be absolute: %q", targetRoot)
	}

	return &Workspace{
		sourceRoot: filepath.Clean(sourceRoot),
		targetRoot: path.Clean(targetRoot),
	}, nil
}

// SourceRoot returns the cleaned host root.
func (m *Workspace) SourceRoot() string {
	return m.sourceRoot
}

// TargetRoot returns the cleaned POSIX target root.
func (m *Workspace) TargetRoot() string {
	return m.targetRoot
}

// TargetPath maps an absolute source path inside the source root to the target.
func (m *Workspace) TargetPath(sourcePath string) (string, error) {
	if !filepath.IsAbs(sourcePath) {
		return "", fmt.Errorf("source path must be absolute: %q", sourcePath)
	}
	relative, err := filepath.Rel(m.sourceRoot, filepath.Clean(sourcePath))
	if err != nil {
		return "", fmt.Errorf("cannot map source path %q: %w", sourcePath, err)
	}
	if !filepath.IsLocal(relative) {
		return "", fmt.Errorf("source path is outside workspace: %q", sourcePath)
	}

	return path.Join(m.targetRoot, filepath.ToSlash(relative)), nil
}

// SourcePath maps an absolute POSIX target path inside the target root to the host.
func (m *Workspace) SourcePath(targetPath string) (string, error) {
	if !path.IsAbs(targetPath) {
		return "", fmt.Errorf("target path must be absolute: %q", targetPath)
	}

	cleaned := path.Clean(targetPath)
	if cleaned == m.targetRoot {
		return m.sourceRoot, nil
	}

	prefix := strings.TrimSuffix(m.targetRoot, "/") + "/"
	relative, ok := strings.CutPrefix(cleaned, prefix)
	if !ok {
		return "", fmt.Errorf("target path is outside workspace: %q", targetPath)
	}
	native := filepath.FromSlash(relative)
	if !filepath.IsLocal(native) || filepath.ToSlash(native) != relative {
		return "", fmt.Errorf("target path cannot be mapped to source: %q", targetPath)
	}

	return filepath.Join(m.sourceRoot, native), nil
}
