package dev

import (
	"context"
	"os/exec"

	"github.com/shopware/shopware-cli/internal/oci"
)

// composeCommand builds a `compose <args...>` command rooted at projectRoot.
// It is the single place the dev package shells out to the OCI runtime's
// compose subcommand, so the working directory is defined once.
func composeCommand(ctx context.Context, projectRoot string, args ...string) *exec.Cmd {
	cmd := oci.FromContext(ctx).ComposeCommand(ctx, args...)
	cmd.Dir = projectRoot
	return cmd
}
