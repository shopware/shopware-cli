package executor

// PHP returns helpers for PHP and common PHP tools.
func (e *Executor) PHP() *PHP {
	return &PHP{executor: e}
}

// PHP builds commands without executing them. PHP and Composer must already be
// available in the target execution environment; the helpers do not install them.
type PHP struct {
	executor *Executor
}

// Run creates a PHP invocation.
func (p *PHP) Run(args ...string) *Command {
	return p.executor.Command("php", args...)
}

// Composer creates a Composer invocation.
func (p *PHP) Composer(args ...string) *Command {
	return p.executor.Command("composer", args...)
}

// Console creates a Shopware console invocation.
func (p *PHP) Console(args ...string) *Command {
	return p.executor.Command("php", append([]string{"bin/console"}, args...)...)
}
