package deployment

import (
	"context"
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
