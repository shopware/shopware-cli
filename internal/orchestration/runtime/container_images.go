package runtime

import (
	"context"
	"io"
)

// ContainerImages manages the selected engine's image store.
// Stdout and stderr are forwarded separately. Nil writers discard their stream.
// Cancellation does not promise rollback.
type ContainerImages interface {
	// InspectImage returns local metadata without pulling. Absence returns ErrNotFound.
	InspectImage(context.Context, string) (ContainerImageState, error)
	// PullImage downloads an image and forwards the client's output.
	PullImage(ctx context.Context, reference string, options PullImageOptions) error
	// BuildImage builds an image and returns its engine-assigned ID.
	// Normal output is forwarded while building. Tags are optional.
	BuildImage(ctx context.Context, options BuildImageOptions) (string, error)
}

// ContainerImageState describes an image in the selected engine.
type ContainerImageState struct {
	// ID identifies the image in the selected engine, not necessarily a registry digest.
	ID string
	// References contains known name:tag and name@digest references.
	References []string
}

// PullImageOptions configures image-pull output.
type PullImageOptions struct {
	Stdout io.Writer // Receives stdout. Nil discards it.
	Stderr io.Writer // Receives stderr. Nil discards it.
}

// BuildImageOptions selects the build inputs, optional tags and output.
type BuildImageOptions struct {
	// ContextDir contains the build context on the CLI host.
	ContextDir string
	// Dockerfile is relative to ContextDir. Empty means Dockerfile.
	Dockerfile string
	// Tags assigns optional name:tag references to the build result.
	Tags   []string
	Stdout io.Writer // Receives stdout. Nil discards it.
	Stderr io.Writer // Receives stderr. Nil discards it.
}
