// Package executor builds and runs commands through an injected runtime.
package executor

import (
	"maps"

	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

// Executor builds commands for the selected runtime.
type Executor struct {
	runtime runtime.Runtime
	dir     string
	env     map[string]string
}

// New creates an executor using the selected runtime.
func New(target runtime.Runtime) *Executor {
	return &Executor{runtime: target}
}

// WithDir returns an executor with the default working directory replaced.
func (e *Executor) WithDir(dir string) *Executor {
	executor := *e
	executor.dir = dir

	return &executor
}

// WithEnv returns an executor with the provided values overlaid on its defaults.
func (e *Executor) WithEnv(env map[string]string) *Executor {
	executor := *e
	executor.env = maps.Clone(e.env)
	if executor.env == nil && len(env) > 0 {
		executor.env = make(map[string]string, len(env))
	}
	maps.Copy(executor.env, env)

	return &executor
}

// Command creates a command with a copied argument list and default settings.
func (e *Executor) Command(name string, args ...string) *Command {
	return &Command{
		executor: e,
		spec: runtime.Command{
			Name: name,
			Args: append([]string(nil), args...),
			Dir:  e.dir,
			Env:  maps.Clone(e.env),
		},
	}
}
