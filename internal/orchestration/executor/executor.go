// Package executor constructs commands and captures output through an injected runtime.
package executor

import (
	"maps"
	"slices"

	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

// Executor builds commands using defaults for the selected target environment.
// It does not discover or install executables. Methods return independent copies
// and may be called concurrently.
type Executor struct {
	target runtime.Runtime
	dir    string
	env    map[string]string
}

// New constructs an executor. The target must be non-nil. Construction performs
// no external operations.
func New(target runtime.Runtime) *Executor {
	return &Executor{target: target}
}

// WithDir returns a copy with the default target working directory replaced.
func (e *Executor) WithDir(dir string) *Executor {
	derived := *e
	derived.dir = dir
	return &derived
}

// WithEnv returns a copy with env overlaid on the default target environment.
func (e *Executor) WithEnv(env map[string]string) *Executor {
	derived := *e
	derived.env = overlayEnv(e.env, env)
	return &derived
}

// Command creates a reusable command with independent arguments and defaults.
func (e *Executor) Command(name string, args ...string) *Command {
	return &Command{
		target: e.target,
		spec: runtime.Command{
			Name: name,
			Args: slices.Clone(args),
			Dir:  e.dir,
			Env:  maps.Clone(e.env),
		},
	}
}

// overlayEnv merges environment overrides without modifying either input.
func overlayEnv(base, overrides map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(overrides))
	maps.Copy(merged, base)
	maps.Copy(merged, overrides)
	return merged
}
