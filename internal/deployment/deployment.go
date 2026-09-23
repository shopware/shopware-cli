// Package deployment owns deployment lifecycles and backend capabilities.
package deployment

import (
	"context"
	"errors"
	"io"
	"time"
)

var ErrNotSupported = errors.New("deployment not supported")

// Deployment identifies an immutable deployment created by a backend. The
// reference is backend-specific, for example an archive path for SSH or an
// application-build ID for Shopware PaaS.
type Deployment struct {
	Reference string `json:"reference"`
	// Name is an optional backend-provided label for human-readable output.
	Name string `json:"name,omitempty"`
}

func (d Deployment) DisplayName() string {
	if d.Name != "" {
		return d.Name
	}
	return d.Reference
}

// Rollout identifies one activation of a deployment. The reference is
// backend-specific, for example an SSH release name or a Shopware PaaS
// application-deployment ID.
type Rollout struct {
	Reference  string     `json:"reference"`
	Deployment Deployment `json:"deployment"`
	Active     bool       `json:"active"`
	// DeployedAt is the activation time, when recorded by the backend.
	DeployedAt *time.Time `json:"deployed_at"`
	// Unchanged means the requested deployment was already active; no new
	// activation was performed and Reference identifies the existing rollout.
	Unchanged bool `json:"unchanged,omitempty"`
}

// Backend owns creation and activation of immutable deployments.
type Backend interface {
	Type() string
	CreateDeployment(ctx context.Context, options CreateOptions) (Deployment, error)
	RolloutDeployment(ctx context.Context, deployment Deployment, output io.Writer) (Rollout, error)
}

type CreateOptions struct {
	OutputPath          string
	WithDevDependencies bool
	ToolVersion         string
}

// DeploymentLogs exposes retained deployment-helper output without requiring
// callers to know where or how a backend stores it.
type DeploymentLogs interface {
	WriteDeploymentLogs(ctx context.Context, deployment Deployment, output io.Writer) error
}

// DeploymentPruneOptions controls retention of distinct successful deployments
// on the target. The active deployment and failed/incomplete releases are
// protected independently of Keep. Cleanup already in progress is resumed.
type DeploymentPruneOptions struct {
	Keep   int  `json:"keep"`
	DryRun bool `json:"dry_run"`
}

// DeploymentPruneResult identifies removed deployments and cached artifacts,
// or the planned removals when DryRun is true.
type DeploymentPruneResult struct {
	Deployments []Deployment `json:"deployments"`
	Artifacts   []string     `json:"artifacts"`
}

type DeploymentPruner interface {
	PruneDeployments(ctx context.Context, options DeploymentPruneOptions) (DeploymentPruneResult, error)
}

// RolloutHistory is an optional capability for backends that can expose
// successful rollouts. ListRollouts returns them in newest-first order.
type RolloutHistory interface {
	ListRollouts(ctx context.Context) ([]Rollout, error)
}
