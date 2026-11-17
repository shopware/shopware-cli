package executor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

var _ runtime.ServiceStack = (*DockerCompose)(nil)

var projectNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// DockerComposeOptions identifies the Compose project and configures lifecycle output.
type DockerComposeOptions struct {
	// Directory is the absolute project directory on the Docker CLI host.
	Directory string
	// ProjectName identifies the Compose project.
	ProjectName string
	// Stdout receives lifecycle command output. Nil discards streamed output.
	Stdout io.Writer
	// Stderr receives lifecycle command diagnostics. Nil discards streamed output.
	Stderr io.Writer
}

// DockerCompose manages a project through a supplied Docker runtime.
type DockerCompose struct {
	executor *Executor
	options  DockerComposeOptions
}

// NewDockerCompose validates project options without running Docker commands.
func NewDockerCompose(commands DockerCommands, options DockerComposeOptions) (*DockerCompose, error) {
	if commands == nil {
		return nil, errors.New("docker command execution is required")
	}
	if !filepath.IsAbs(options.Directory) || strings.ContainsRune(options.Directory, 0) {
		return nil, errors.New("absolute project directory is required")
	}
	if !projectNamePattern.MatchString(options.ProjectName) {
		return nil, errors.New("invalid project name")
	}
	return &DockerCompose{executor: New(dockerCommandRuntime{commands}).WithDir(options.Directory), options: options}, nil
}

func (p *DockerCompose) command(args ...string) *Command {
	prefix := []string{"--project-directory", p.options.Directory, "--project-name", p.options.ProjectName}
	return p.executor.Command("compose", append(prefix, args...)...)
}

// query captures machine-readable output without forwarding it to lifecycle writers.
func (p *DockerCompose) query(ctx context.Context, args ...string) ([]byte, error) {
	result, err := p.command(args...).Run(ctx, runtime.Options{Capture: runtime.CaptureSeparate})
	if err == nil && result.ExitCode != 0 {
		err = fmt.Errorf("compose exited with code %d", result.ExitCode)
	}
	if err != nil {
		if diagnostic := strings.TrimSpace(string(result.Output.Stderr)); diagnostic != "" {
			return nil, fmt.Errorf("cannot query compose %s: %s: %w", args[0], diagnostic, err)
		}
		return nil, fmt.Errorf("cannot query compose %s: %w", args[0], err)
	}
	return result.Output.Stdout, nil
}

// CheckAvailable checks whether Docker Compose is available.
func (p *DockerCompose) CheckAvailable(ctx context.Context) error {
	return p.lifecycle(ctx, "version")
}

// Up brings up services without waiting for readiness.
func (p *DockerCompose) Up(ctx context.Context) error {
	return p.lifecycle(ctx, "up", "--detach")
}

// Stop stops services while retaining project resources.
func (p *DockerCompose) Stop(ctx context.Context) error {
	return p.lifecycle(ctx, "stop")
}

// DockerComposeRemoveOptions controls which resources are removed with a project.
type DockerComposeRemoveOptions struct {
	// RemoveVolumes also removes named volumes.
	RemoveVolumes bool
}

// Remove removes project resources and optionally named volumes.
func (p *DockerCompose) Remove(ctx context.Context, options DockerComposeRemoveOptions) error {
	args := []string{"down"}
	if options.RemoveVolumes {
		args = append(args, "--volumes")
	}
	return p.lifecycle(ctx, args...)
}

func (p *DockerCompose) lifecycle(ctx context.Context, args ...string) error {
	result, err := p.command(args...).Run(ctx, runtime.Options{
		Stdout:  p.options.Stdout,
		Stderr:  p.options.Stderr,
		Capture: runtime.CaptureCombined,
	})
	if err == nil {
		switch {
		case result.ExitCode < 0:
			err = &runtime.ExecutionError{Stage: runtime.StageWait, Err: errors.New("unknown exit status")}
		case result.ExitCode > 0:
			err = &runtime.ExitError{Code: result.ExitCode}
		}
	}
	if err == nil {
		return nil
	}
	if output := strings.TrimSpace(string(result.Output.Combined)); output != "" {
		return fmt.Errorf("cannot run compose %s: %s: %w", args[0], output, err)
	}
	return fmt.Errorf("cannot run compose %s: %w", args[0], err)
}

// Service binds command execution to a named service without starting it.
func (p *DockerCompose) Service(name string) (runtime.Runtime, error) {
	if name == "" || strings.HasPrefix(name, "-") || strings.ContainsRune(name, 0) {
		return nil, errors.New("invalid service name")
	}
	return &serviceRuntime{project: p, name: name}, nil
}

type serviceRuntime struct {
	project *DockerCompose
	name    string
}

// Start runs a noninteractive service command without starting the project.
// Unsuccessful exits have unknown workload status because Compose also reports
// setup failures. Stopping the client does not guarantee service-process termination.
func (s *serviceRuntime) Start(ctx context.Context, command runtime.Command, options runtime.Options) (runtime.Process, error) {
	if options.TTY {
		return nil, startError(runtime.ErrUnsupported)
	}
	if command.Name == "" || strings.HasPrefix(command.Name, "-") || strings.ContainsRune(command.Name, 0) {
		return nil, startError(errors.New("invalid command name"))
	}
	args := []string{"exec", "-T"}
	keys := slices.Sorted(maps.Keys(command.Env))
	for _, key := range keys {
		args = append(args, "--env", key+"="+command.Env[key])
	}
	if command.Dir != "" {
		args = append(args, "--workdir", command.Dir)
	}
	args = append(args, "--", s.name, command.Name)
	args = append(args, command.Args...)
	process, err := s.project.command(args...).Start(ctx, options)
	if err != nil {
		return nil, composeClientError{err}
	}
	return &serviceProcess{process: process}, nil
}

func startError(err error) error {
	return &runtime.ExecutionError{Stage: runtime.StageStart, Err: err}
}

type serviceProcess struct{ process runtime.Process }

// Compose cannot distinguish a service exit from an exec setup failure.
func (p *serviceProcess) Wait() (runtime.Result, error) {
	result, err := p.process.Wait()
	result.Usage = nil
	if err != nil || result.ExitCode != 0 {
		if err == nil {
			err = &runtime.ExitError{Code: result.ExitCode}
		}
		result.ExitCode = -1
		return result, &runtime.ExecutionError{Stage: runtime.StageWait, Err: composeClientError{err}}
	}
	return result, nil
}

func (p *serviceProcess) Stop(context.Context) error {
	return &runtime.ExecutionError{Stage: runtime.StageStop, Err: runtime.ErrUnsupported}
}

func (p *serviceProcess) Kill(context.Context) error {
	return &runtime.ExecutionError{Stage: runtime.StageStop, Err: runtime.ErrUnsupported}
}

// DockerCommands executes Docker CLI subcommands on the selected host.
type DockerCommands interface {
	ExecDocker(context.Context, runtime.Command, runtime.Options) (runtime.Process, error)
}

type dockerCommandRuntime struct{ commands DockerCommands }

func (b dockerCommandRuntime) Start(ctx context.Context, command runtime.Command, options runtime.Options) (runtime.Process, error) {
	return b.commands.ExecDocker(ctx, command, options)
}

type composeClientError struct{ err error }

func (e composeClientError) Error() string { return e.err.Error() }
func (e composeClientError) Is(target error) bool {
	if _, ok := target.(*runtime.ExitError); ok {
		return false
	}
	return errors.Is(e.err, target)
}
func (e composeClientError) As(target any) bool {
	if _, ok := target.(**runtime.ExitError); ok {
		return false
	}
	return errors.As(e.err, target)
}
