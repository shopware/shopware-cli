package deployment

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ReferenceCompleter optionally discovers backend-specific deployment references.
type ReferenceCompleter interface {
	CompleteDeploymentReferences(ctx context.Context, prefix string) ([]string, error)
}

func (s *SSH) CompleteDeploymentReferences(ctx context.Context, prefix string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(s.root, ".shopware-cli", "deployments"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	references := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(entry.Name(), ".tar.gz") {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		reference := strings.TrimSuffix(entry.Name(), ".tar.gz")
		if strings.HasPrefix(reference, prefix) {
			references = append(references, reference)
		}
	}
	slices.Sort(references)
	return references, nil
}

// resolveDeploymentArchive expands a generated deployment name to its archive.
// Explicit paths and opaque references without a matching local archive remain
// unchanged.
func resolveDeploymentArchive(root, reference string) string {
	if reference == "" || filepath.Base(reference) != reference {
		return reference
	}
	filename := reference
	if filepath.Ext(filename) == "" {
		filename += ".tar.gz"
	}
	candidate := filepath.Join(root, ".shopware-cli", "deployments", filename)
	info, err := os.Lstat(candidate)
	if err == nil && info.Mode().IsRegular() {
		return candidate
	}
	return reference
}
