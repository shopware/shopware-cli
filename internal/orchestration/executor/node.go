package executor

// Node returns helpers for Node.js and npm.
func (e *Executor) Node() *Node {
	return &Node{executor: e}
}

// Node builds commands without executing them. Node.js and npm must already be
// available in the target execution environment; the helpers do not install them.
type Node struct {
	executor *Executor
}

// Run creates a Node.js invocation.
func (n *Node) Run(args ...string) *Command {
	return n.executor.Command("node", args...)
}

// NPM creates an npm invocation.
func (n *Node) NPM(args ...string) *Command {
	return n.executor.Command("npm", args...)
}
