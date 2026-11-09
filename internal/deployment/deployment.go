// Package deployment owns deployment artifacts and backend capabilities.
package deployment

import (
	"context"
	"errors"
)

var ErrNotSupported = errors.New("deployment not supported")

// Deployment identifies an immutable, backend-specific artifact.
type Deployment struct {
	Reference string `json:"reference"`
}

// Backend creates immutable deployments for an environment.
type Backend interface {
	Type() string
	CreateDeployment(context.Context, CreateOptions) (Deployment, error)
}

type CreateOptions struct {
	OutputPath          string
	WithDevDependencies bool
	ToolVersion         string
}
