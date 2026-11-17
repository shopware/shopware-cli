package docker

import (
	"context"
	"errors"

	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

// A Docker CLI exit describes the supporting process, not the workload. Keep
// other error identities available without exposing that exit as a workload exit.
func hideClientExit(err error) error {
	var exitErr *runtime.ExitError
	if errors.As(err, &exitErr) {
		return clientError{err: err}
	}
	return err
}

type clientError struct{ err error }

func (e clientError) Error() string { return e.err.Error() }

func (e clientError) Is(target error) bool {
	if _, isExit := target.(*runtime.ExitError); isExit {
		return false
	}
	return errors.Is(e.err, target)
}

func (e clientError) As(target any) bool {
	if _, isExit := target.(**runtime.ExitError); isExit {
		return false
	}
	return errors.As(e.err, target)
}

// ExecDocker runs a Docker subcommand on the selected host runtime.
// Command directories and environment overrides apply to the Docker client.
func (c *Runtime) ExecDocker(ctx context.Context, command runtime.Command, options runtime.Options) (runtime.Process, error) {
	if c.host == nil {
		return nil, &runtime.ExecutionError{Stage: runtime.StageStart, Err: errors.New("docker command runtime is required")}
	}
	if !validName(command.Name) {
		return nil, &runtime.ExecutionError{Stage: runtime.StageStart, Err: errors.New("invalid docker subcommand")}
	}
	command.Args = append([]string{command.Name}, command.Args...)
	command.Name = "docker"
	return c.host.Start(ctx, command, options)
}
