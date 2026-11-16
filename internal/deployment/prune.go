package deployment

import "context"

// Prune previews cleanup before confirmation when confirm is provided. A nil
// result means cleanup was declined. Dry runs never prompt.
func Prune(ctx context.Context, backend DeploymentPruner, options DeploymentPruneOptions, confirm func(DeploymentPruneResult) (bool, error)) (*DeploymentPruneResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !options.DryRun && confirm != nil {
		previewOptions := options
		previewOptions.DryRun = true
		preview, err := backend.PruneDeployments(ctx, previewOptions)
		if err != nil {
			return nil, err
		}
		confirmed, err := confirm(preview)
		if err != nil || !confirmed {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result, err := backend.PruneDeployments(ctx, options)
	return &result, err
}
