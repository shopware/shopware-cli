package executor

import (
	"context"
	"regexp"

	"github.com/shyim/go-version"
)

// phpVersionBanner captures the version from `php -v` output such as
// "PHP 8.3.6-1ubuntu1 (cli) ...". PHP prints startup warnings before the
// banner, so the pattern matches at the start of any line.
var phpVersionBanner = regexp.MustCompile(`(?m)^PHP\s+(\d+\.\d+(?:\.\d+)?)`)

// PHP builds PHP and Composer commands with the executor's defaults. The tools
// must already exist in the target environment.
type PHP struct {
	executor *Executor
}

// PHP returns command builders for PHP and Composer.
func (e *Executor) PHP() PHP {
	return PHP{executor: e}
}

// Command creates a php invocation, for example of a script and its arguments.
func (p PHP) Command(args ...string) *Command {
	return p.executor.Command("php", args...)
}

// Composer creates a composer invocation.
func (p PHP) Composer(args ...string) *Command {
	return p.executor.Command("composer", args...)
}

// Version returns the version of the php executable that Command runs.
func (p PHP) Version(ctx context.Context) (*version.Version, error) {
	return probeVersion(ctx, p.Command("-v"), "PHP", phpVersionBanner)
}
