//go:build deployment

package project

import (
	"context"
	"io"

	"github.com/shopware/shopware-cli/internal/deployment"
)

// A backend with no optional capabilities. Unexpected lifecycle calls fail tests
// rather than delegating to a nil embedded general-purpose executor.
type deploymentTestBackend struct{}

func (deploymentTestBackend) Type() string { return "fake" }

func (deploymentTestBackend) CreateDeployment(context.Context, deployment.CreateOptions) (deployment.Deployment, error) {
	panic("unexpected create")
}

func (deploymentTestBackend) RolloutDeployment(context.Context, deployment.Deployment, io.Writer) (deployment.Rollout, error) {
	panic("unexpected rollout")
}
