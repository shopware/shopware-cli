package executor

import "context"

// Deployment identifies an immutable deployment created by a backend. The
// reference is backend-specific, for example an archive path for SSH or an
// application-build ID for Shopware PaaS.
type Deployment struct {
	Reference string
}

// Rollout identifies one activation of a deployment. The reference is
// backend-specific, for example an SSH release name or a Shopware PaaS
// application-deployment ID.
type Rollout struct {
	Reference  string
	Deployment Deployment
	Active     bool
}

// DeploymentExecutor separates creating an immutable deployment from rolling
// that exact deployment out. Implementations may build a local artifact (SSH)
// or delegate both operations to another CLI (Shopware PaaS).
type DeploymentExecutor interface {
	CreateDeployment(ctx context.Context) (Deployment, error)
	RolloutDeployment(ctx context.Context, deployment Deployment) (Rollout, error)
}

// RolloutHistory is an optional capability for executors that can expose
// successful rollouts. ListRollouts returns them in newest-first order.
type RolloutHistory interface {
	ListRollouts(ctx context.Context) ([]Rollout, error)
}
