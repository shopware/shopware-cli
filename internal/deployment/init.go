package deployment

import (
	"context"
	"errors"
	"io"
)

// ErrInitializationCanceled indicates that the user declined the final write.
var ErrInitializationCanceled = errors.New("deployment initialization canceled")

// InitializationConfig supplies the same runtime and optional first-install values as the setup form.
// Keep files containing these values private, or pass them over stdin.
type InitializationConfig struct {
	RuntimeValues map[string]string `json:"runtime_values"`
	InstallValues map[string]string `json:"install_values,omitempty"`
}

// InitializationOptions selects scripted setup when Config is provided.
type InitializationOptions struct {
	Config *InitializationConfig
}

// Initializer bootstraps a deployment environment interactively or from supplied values.
type Initializer interface {
	InitializeDeployment(ctx context.Context, options InitializationOptions) error
}

// InitializationGuide describes the remaining setup after initialization succeeds.
type InitializationGuide interface {
	WriteInitializationNextSteps(environment string, output io.Writer) error
}
