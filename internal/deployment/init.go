package deployment

import "context"

// Initializer is an optional capability for deployment backends that can
// interactively bootstrap their target environment.
type Initializer interface {
	InitializeDeployment(ctx context.Context) error
}
