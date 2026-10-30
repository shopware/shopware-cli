// Package deployment owns deployment lifecycles and backend capabilities.
package deployment

import (
	"context"
	"errors"
	"io"
	"time"
)

var ErrNotSupported = errors.New("deployment not supported")

// Deployment identifies an immutable, backend-specific artifact.
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

// Rollout identifies one activation of a deployment.
type Rollout struct {
	Host       string     `json:"host,omitempty"`
	Reference  string     `json:"reference"`
	Deployment Deployment `json:"deployment"`
	Active     bool       `json:"active"`
	// DeployedAt is the activation time, when recorded by the backend.
	DeployedAt *time.Time `json:"deployed_at"`
	// Unchanged returns the existing rollout without performing an activation.
	Unchanged bool `json:"unchanged,omitempty"`
}

// Backend creates immutable deployments for an environment.
type Backend interface {
	Type() string
	CreateDeployment(ctx context.Context, options CreateOptions) (Deployment, error)
}

type CreateOptions struct {
	OutputPath          string
	WithDevDependencies bool
	ToolVersion         string
}

// DeploymentLogs streams retained helper output.
type DeploymentLogs interface {
	WriteDeploymentLogs(ctx context.Context, deployment Deployment, output io.Writer) error
}

// RolloutHistory lists successful retained rollouts, newest first.
type RolloutHistory interface {
	ListRollouts(ctx context.Context) ([]Rollout, error)
}

// DeploymentActivator reactivates a retained deployment without running migrations.
type DeploymentActivator interface {
	ActivateDeployment(ctx context.Context, deployment Deployment, output io.Writer) (Rollout, error)
}
