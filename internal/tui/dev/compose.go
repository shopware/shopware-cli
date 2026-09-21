package dev

import (
	"context"

	"github.com/shopware/shopware-cli/internal/oci"
)

// composeCommand builds a `compose <args...>` command rooted at projectRoot.
// It is the single place the dev package shells out to the OCI runtime's
// compose subcommand, so the working directory is defined once.
func composeCommand(ctx context.Context, projectRoot string, args ...string) oci.Cmd {
	cmd := oci.FromContext(ctx).ComposeCommand(ctx, args...)
	cmd.SetDir(projectRoot)
	return cmd
}
