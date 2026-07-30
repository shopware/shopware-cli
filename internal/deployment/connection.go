package deployment

import (
	"context"
	"io"
)

// Connection is a shell on the deployment target. It is an interface so the
// release logic can be tested without a real server.
type Connection interface {
	// Run executes a command remotely and returns its combined output
	Run(ctx context.Context, command string) (string, error)
	// Stream executes a command remotely with the given reader attached to stdin
	Stream(ctx context.Context, command string, stdin io.Reader) error
	// Close terminates the connection
	Close() error
}
