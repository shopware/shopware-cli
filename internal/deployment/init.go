package deployment

import (
	"context"
	"errors"
	"io"
)

// ErrInitializationCanceled indicates that the user declined the final write.
var ErrInitializationCanceled = errors.New("deployment initialization canceled")

// Initializer bootstraps a deployment environment interactively.
type Initializer interface {
	InitializeDeployment(ctx context.Context) error
}

// InitializationGuide describes the remaining setup after initialization succeeds.
type InitializationGuide interface {
	WriteInitializationNextSteps(environment string, output io.Writer) error
}
