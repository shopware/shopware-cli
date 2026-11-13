// Package compose provides a minimal Docker Compose stack provider.
package compose

import (
	"context"
	"errors"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/shopware/shopware-cli/internal/orchestration/executor"
	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
	"github.com/shopware/shopware-cli/internal/orchestration/stack"
)

var projectNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// Options identifies the Compose project.
type Options struct {
	// Directory is the absolute project directory on the backend.
	Directory string
	// ProjectName identifies the Compose project.
	ProjectName string
}

// Provider runs Compose commands through an injected backend.
type Provider struct {
	executor *executor.Executor
	options  Options
}

// New creates a provider without discovering Compose configuration.
func New(backend runtime.Backend, options Options) (*Provider, error) {
	if backend == nil {
		return nil, errors.New("backend is required")
	}
	if !filepath.IsAbs(options.Directory) || strings.ContainsRune(options.Directory, 0) {
		return nil, errors.New("absolute project directory is required")
	}
	if !projectNamePattern.MatchString(options.ProjectName) {
		return nil, errors.New("invalid project name")
	}
	return &Provider{executor: executor.New(backend).WithDir(options.Directory), options: options}, nil
}

func (p *Provider) command(args ...string) *executor.Command {
	prefix := []string{"compose", "--project-directory", p.options.Directory, "--project-name", p.options.ProjectName}
	return p.executor.Command("docker", append(prefix, args...)...)
}

// Start brings up services without waiting for readiness.
func (p *Provider) Start(ctx context.Context, options runtime.Options) (runtime.Result, error) {
	return p.lifecycle(ctx, options, "up", "--detach")
}

// Stop stops services while retaining stack resources.
func (p *Provider) Stop(ctx context.Context, options runtime.Options) (runtime.Result, error) {
	return p.lifecycle(ctx, options, "stop")
}

// Remove removes stack resources and optionally named volumes.
func (p *Provider) Remove(ctx context.Context, remove stack.RemoveOptions, options runtime.Options) (runtime.Result, error) {
	args := []string{"down"}
	if remove.RemoveVolumes {
		args = append(args, "--volumes")
	}
	return p.lifecycle(ctx, options, args...)
}

func (p *Provider) lifecycle(ctx context.Context, options runtime.Options, args ...string) (runtime.Result, error) {
	if options.TTY {
		return runtime.Result{ExitCode: -1}, startError(runtime.ErrUnsupported)
	}
	return p.command(args...).Run(ctx, options)
}

// Exec starts a noninteractive service command without starting the stack.
// Unsuccessful exits have unknown workload status because Compose also reports
// setup failures. Stopping the client does not guarantee service-process termination.
func (p *Provider) Exec(ctx context.Context, service string, command runtime.Command, options runtime.Options) (runtime.Process, error) {
	if options.TTY {
		return nil, startError(runtime.ErrUnsupported)
	}
	if service == "" || strings.HasPrefix(service, "-") || strings.ContainsRune(service, 0) {
		return nil, startError(errors.New("invalid service name"))
	}
	if command.Name == "" || strings.HasPrefix(command.Name, "-") || strings.ContainsRune(command.Name, 0) {
		return nil, startError(errors.New("invalid command name"))
	}
	args := []string{"exec", "-T"}
	keys := make([]string, 0, len(command.Env))
	for key := range command.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		args = append(args, "--env", key+"="+command.Env[key])
	}
	if command.Dir != "" {
		args = append(args, "--workdir", command.Dir)
	}
	args = append(args, "--", service, command.Name)
	args = append(args, command.Args...)
	process, err := p.command(args...).Start(ctx, options)
	if err != nil {
		return nil, err
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
		return result, &runtime.ExecutionError{Stage: runtime.StageWait, Err: err}
	}
	return result, nil
}

func (p *serviceProcess) Stop(ctx context.Context) error { return p.process.Stop(ctx) }
