package deployment

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Candidate is a backend-discovered deployment available for rollout or rollback.
type Candidate struct {
	Deployment Deployment `json:"deployment"`
	// BuiltAt uses the archive modification time for locally packaged SSH builds.
	BuiltAt    *time.Time `json:"built_at,omitempty"`
	CreatedAt  *time.Time `json:"created_at,omitempty"`
	DeployedAt *time.Time `json:"deployed_at"`
	Active     bool       `json:"active"`
}

type CandidateProvider interface {
	RolloutCandidates(ctx context.Context) ([]Candidate, error)
	RollbackCandidates(ctx context.Context) ([]Candidate, error)
}

func (s *SSH) RolloutCandidates(ctx context.Context) ([]Candidate, error) {
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
	candidates := make([]Candidate, 0, len(entries))
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(entry.Name(), ".tar.gz") {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		reference := strings.TrimSuffix(entry.Name(), ".tar.gz")
		builtAt := info.ModTime().UTC()
		candidates = append(candidates, Candidate{
			Deployment: Deployment{Reference: reference, Name: reference},
			BuiltAt:    &builtAt,
		})
	}
	// Prefer the newest build; use deployment names to break timestamp ties.
	slices.SortFunc(candidates, func(a, b Candidate) int {
		if order := b.BuiltAt.Compare(*a.BuiltAt); order != 0 {
			return order
		}
		return strings.Compare(a.Deployment.Name, b.Deployment.Name)
	})
	return candidates, nil
}

// resolveDeploymentArchive resolves local names and leaves other references unchanged.
func resolveDeploymentArchive(root, reference string) string {
	if reference == "" || filepath.Base(reference) != reference {
		return reference
	}
	filename := reference
	if !strings.HasSuffix(filename, ".tar.gz") {
		filename += ".tar.gz"
	}
	candidate := filepath.Join(root, ".shopware-cli", "deployments", filename)
	info, err := os.Lstat(candidate)
	if err == nil && info.Mode().IsRegular() {
		return candidate
	}
	return reference
}
