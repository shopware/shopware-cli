// Package executor provides a small command API over runtime backends.
package executor

import (
	"maps"

	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

// Executor builds commands for a runtime backend.
type Executor struct {
	backend runtime.Backend
	dir     string
	env     map[string]string
}

// New creates an executor backed by backend.
func New(backend runtime.Backend) *Executor {
	return &Executor{backend: backend}
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
