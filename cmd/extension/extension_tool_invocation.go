package extension

import (
	"slices"
	"sort"

	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier"
)

func extensionToolInvocationStatuses(all, selected verifier.ToolList) []validation.ToolInvocationStatus {
	statuses := make([]validation.ToolInvocationStatus, 0, len(all))
	for _, tool := range all {
		status := validation.ToolInvocationStatus{Name: tool.Name(), Status: "skipped", Reason: "not selected by --only"}
		if slices.ContainsFunc(selected, func(selected verifier.Tool) bool { return selected.Name() == tool.Name() }) {
			status.Status = "invoked"
			status.Reason = ""
		}
		statuses = append(statuses, status)
	}
	sort.Slice(statuses, func(i, j int) bool { return statuses[i].Name < statuses[j].Name })
	return statuses
}
