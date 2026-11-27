package executor

import (
	"context"
	"regexp"

	"github.com/shyim/go-version"
)

// nodeVersionOutput captures the version from `node -v` output such as
// "v20.11.1". Nightly and release candidate suffixes are left out, as they do
// not follow one format.
var nodeVersionOutput = regexp.MustCompile(`^v(\d+\.\d+\.\d+)`)

// Node builds Node.js and npm commands with the executor's defaults. The tools
// must already exist in the target environment.
type Node struct {
	executor *Executor
}

// Node returns command builders for Node.js and npm.
func (e *Executor) Node() Node {
	return Node{executor: e}
}

// Command creates a node invocation, for example of a script and its arguments.
func (n Node) Command(args ...string) *Command {
	return n.executor.Command("node", args...)
}

// NPM creates an npm invocation.
func (n Node) NPM(args ...string) *Command {
	return n.executor.Command("npm", args...)
}

// Version returns the version of the node executable that Command runs.
func (n Node) Version(ctx context.Context) (*version.Version, error) {
	return probeVersion(ctx, n.Command("-v"), "Node.js", nodeVersionOutput)
}
