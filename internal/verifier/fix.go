package verifier

import (
	"context"
	"slices"

	"github.com/shopware/shopware-cli/internal/validation"
)

// FollowUpFixTools returns the fixers that must run after the selected fixers
// have finished.
func FollowUpFixTools(selected ToolList[FixTool]) ToolList[FixTool] {
	if slices.ContainsFunc(selected, func(tool FixTool) bool { return tool.Name() == Rector{}.Name() }) {
		return ToolList[FixTool]{CookieListenerWiring{}}
	}

	return nil
}

// RunFixTools runs the selected fixers in parallel and afterwards their
// follow-up fixers, which depend on the files the parallel fixers changed.
func RunFixTools(ctx context.Context, tools ToolList[FixTool], skipped []ToolResult, config ToolConfig, progress *Progress) []validation.ToolReport {
	fix := func(tool FixTool, execution *ToolExecution) ToolSummary {
		return tool.Fix(ctx, config, execution)
	}

	results := RunTools(ctx, tools, skipped, config, progress, fix)
	if followUps := FollowUpFixTools(tools); len(followUps) > 0 && ctx.Err() == nil {
		results = append(results, RunTools(ctx, followUps, nil, config, progress, fix)...)
	}

	return results
}
