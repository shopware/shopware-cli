package bootstrap

import (
	"context"

	"github.com/spf13/cobra"
)

// Step defines a bootstrap function that is executed during command setup.
//
// A Step receives the current Context and the *cobra.Command being executed,
// performs some initialization (e.g. load config, set up logging), and returns
// a new Context enriched with additional values.
//
// If the Step fails, it should return the unchanged Context together with
// an error. When composed with Run, multiple Steps can be executed in order,
// threading the Context through the setup pipeline.
type Step func(context.Context, *cobra.Command) (context.Context, error)

// Run executes a sequence of bootstrap Steps in order, passing the Context
// through each step.
//
// Each Step receives the current Context and *cobra.Command, and may return
// a new Context enriched with additional values (e.g. config, logger).
// If any step returns an error, Run stops immediately and returns the
// Context produced by that step together with the error. If all steps
// succeed, the final accumulated Context is returned.
func Run(ctx context.Context, cmd *cobra.Command, steps ...Step) (context.Context, error) {
	var err error
	// iterate steps
	for _, step := range steps {
		ctx, err = step(ctx, cmd)
		if err != nil {
			// on error skip remaining steps
			return ctx, err
		}
	}
	return ctx, nil
}
