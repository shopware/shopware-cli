package docker

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

// Container binds execution to an existing container without accessing the engine.
// Execution does not change the container's lifecycle. Stop and Kill are
// unsupported because closing the client does not guarantee workload termination.
func (c *Runtime) Container(id string) (runtime.Runtime, error) {
	if !validName(id) {
		return nil, errors.New("invalid container name")
	}
	return &containerRuntime{docker: c, id: id}, nil
}

type containerRuntime struct {
	docker *Runtime
	id     string
}

func (c *containerRuntime) Start(ctx context.Context, command runtime.Command, options runtime.Options) (runtime.Process, error) {
	if !validName(command.Name) {
		return nil, &runtime.ExecutionError{Stage: runtime.StageStart, Err: errors.New("invalid command name")}
	}
	if options.TTY {
		return nil, &runtime.ExecutionError{Stage: runtime.StageStart, Err: runtime.ErrUnsupported}
	}
	args := []string{"exec"}
	if options.Stdin != nil {
		args = append(args, "-i")
	}
	if command.Dir != "" {
		args = append(args, "--workdir", command.Dir)
	}
	for key := range command.Env {
		if !validEnvKey(key) {
			return nil, &runtime.ExecutionError{Stage: runtime.StageStart, Err: fmt.Errorf("invalid container environment key %q", key)}
		}
	}
	keys := slices.Sorted(maps.Keys(command.Env))
	for _, key := range keys {
		args = append(args, "--env", key+"="+command.Env[key])
	}
	args = append(args, "--", c.id, command.Name)
	args = append(args, command.Args...)
	client, err := c.docker.ExecDocker(ctx, runtime.Command{Name: "container", Args: args}, options)
	if err != nil {
		return nil, &runtime.ExecutionError{Stage: runtime.StageStart, Err: hideClientExit(err)}
	}
	return &containerProcess{client}, nil
}

type containerProcess struct{ client runtime.Process }

func (p *containerProcess) Wait() (runtime.Result, error) {
	result, err := p.client.Wait()
	result.Usage = nil
	if err != nil || result.ExitCode != 0 {
		if err == nil {
			err = fmt.Errorf("docker client exited with code %d", result.ExitCode)
		}
		result.ExitCode = -1
		return result, &runtime.ExecutionError{Stage: runtime.StageWait, Err: hideClientExit(err)}
	}
	return result, nil
}
func (p *containerProcess) Stop(context.Context) error {
	return &runtime.ExecutionError{Stage: runtime.StageStop, Err: runtime.ErrUnsupported}
}

func (p *containerProcess) Kill(context.Context) error {
	return &runtime.ExecutionError{Stage: runtime.StageStop, Err: runtime.ErrUnsupported}
}
