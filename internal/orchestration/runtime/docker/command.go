package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/shopware/shopware-cli/internal/orchestration/executor"
	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

// command retains Docker diagnostics without treating the client's exit as a workload exit.
func (d *Runtime) command(ctx context.Context, args ...string) (string, string, error) {
	if d.host == nil {
		return "", "", errors.New("docker command runtime is required")
	}
	result, err := executor.New(dockerCommandRuntime{d}).Command(args[0], args[1:]...).
		Run(ctx, runtime.Options{Capture: runtime.CaptureSeparate})
	out, diagnostic := string(result.Output.Stdout), strings.TrimSpace(string(result.Output.Stderr))
	if result.ExitCode != 0 || err != nil {
		var cause error
		if err != nil {
			cause = hideClientExit(err)
		}
		if result.ExitCode < 0 {
			cause = errors.Join(cause, errors.New("docker client exit status is unknown"))
		} else if result.ExitCode != 0 {
			cause = errors.Join(cause, fmt.Errorf("docker client exited with code %d", result.ExitCode))
		}
		if ctx.Err() != nil && !errors.Is(cause, ctx.Err()) {
			cause = errors.Join(cause, ctx.Err())
		}
		if diagnostic != "" {
			cause = fmt.Errorf("%w: %s", cause, diagnostic)
		}
		if err != nil {
			if exit, ok := err.(*runtime.ExitError); !ok || exit.Code != result.ExitCode {
				diagnostic = ""
			}
		}
		if result.ExitCode <= 0 || result.ExitCode >= 125 {
			diagnostic = ""
		}
		return out, diagnostic, cause
	}
	if ctx.Err() != nil {
		return out, diagnostic, ctx.Err()
	}
	return out, diagnostic, nil
}

// run streams Docker output and preserves errors from the supporting client.
func (d *Runtime) run(ctx context.Context, stdout, stderr io.Writer, args ...string) error {
	if d.host == nil {
		return errors.New("docker command runtime is required")
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	var diagnostic limitedDiagnostic
	result, err := executor.New(dockerCommandRuntime{d}).Command(args[0], args[1:]...).Run(ctx, runtime.Options{
		Stdout: stdout, Stderr: io.MultiWriter(stderr, &diagnostic),
	})
	if err == nil && result.ExitCode == 0 && ctx.Err() == nil {
		return nil
	}
	var cause error
	if err != nil {
		cause = hideClientExit(err)
	}
	if result.ExitCode < 0 {
		cause = errors.Join(cause, errors.New("docker client exit status is unknown"))
	} else if result.ExitCode != 0 {
		cause = errors.Join(cause, fmt.Errorf("docker client exited with code %d", result.ExitCode))
	}
	if ctx.Err() != nil && !errors.Is(cause, ctx.Err()) {
		cause = errors.Join(cause, ctx.Err())
	}
	if message := strings.TrimSpace(diagnostic.buffer.String()); message != "" {
		return fmt.Errorf("%w: %s", cause, message)
	}
	return cause
}

// limitedDiagnostic bounds retained diagnostics while forwarding continues.
type limitedDiagnostic struct{ buffer bytes.Buffer }

func (d *limitedDiagnostic) Write(p []byte) (int, error) {
	const limit = 4096
	if remaining := limit - d.buffer.Len(); remaining > 0 {
		_, _ = d.buffer.Write(p[:min(len(p), remaining)])
	}
	return len(p), nil
}

// absent accepts only Docker diagnostics that establish the requested resource is missing.
func absent(diagnostic, kind, verb, name string) bool {
	s := strings.TrimSpace(diagnostic)
	if kind == "container" && verb == "start" {
		s = strings.TrimSuffix(s, "\nfailed to start containers: "+name)
	}
	if kind == "network" && verb == "rm" {
		s = strings.TrimSuffix(s, "\nexit status 1")
	}
	return s == "Error response from daemon: No such "+kind+": "+name || s == "Error: No such "+kind+": "+name || (kind == "network" && s == "Error response from daemon: network "+name+" not found")
}

func (d *Runtime) inspect(ctx context.Context, kind, name string, dst any) error {
	if !validName(name) {
		return errors.New("invalid resource name")
	}
	out, diag, err := d.command(ctx, kind, "inspect", "--", name)
	if err != nil {
		if absent(diag, kind, "inspect", name) && ctx.Err() == nil {
			return errors.Join(fmt.Errorf("%w: %s", runtime.ErrNotFound, name), err)
		}
		return fmt.Errorf("cannot inspect %s: %w", kind, err)
	}
	if err = json.Unmarshal([]byte(out), dst); err != nil {
		return fmt.Errorf("cannot decode %s inspection: %w", kind, err)
	}
	return nil
}

// dockerCommandRuntime binds shared capture to the selected Docker connection.
type dockerCommandRuntime struct{ docker *Runtime }

func (b dockerCommandRuntime) Start(ctx context.Context, command runtime.Command, options runtime.Options) (runtime.Process, error) {
	return b.docker.ExecDocker(ctx, command, options)
}
