package extension

import (
	"slices"
	"sort"

	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier"
)

func extensionToolInvocationStatuses[T verifier.Tool](all, requested, selected verifier.ToolList[T]) []validation.ToolInvocationStatus {
	statuses := make([]validation.ToolInvocationStatus, 0, len(all))
	for _, tool := range all {
		status := validation.ToolInvocationStatus{Name: tool.Name(), Status: "skipped", Reason: "not selected by --only"}
		switch {
		case slices.ContainsFunc(selected, func(selected T) bool { return selected.Name() == tool.Name() }):
			status.Status = "invoked"
			status.Reason = ""
		case slices.ContainsFunc(requested, func(requested T) bool { return requested.Name() == tool.Name() }):
			status.Reason = "excluded by --exclude"
		}
		statuses = append(statuses, status)
	}
	sort.Slice(statuses, func(i, j int) bool { return statuses[i].Name < statuses[j].Name })
	return statuses
}
