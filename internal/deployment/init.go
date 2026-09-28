package deployment

import "context"

// Initializer bootstraps a deployment environment interactively.
type Initializer interface {
	InitializeDeployment(ctx context.Context) error
}
