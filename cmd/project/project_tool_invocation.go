package project

import (
	"fmt"
	"slices"
	"sort"

	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier"
)

func projectToolInvocationStatuses[T verifier.Tool](all, requested, selected verifier.ToolList[T]) []validation.ToolInvocationStatus {
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

func selectProjectTools[T verifier.Tool](all verifier.ToolList[T], only, exclude, kind string) (verifier.ToolList[T], []validation.ToolInvocationStatus, error) {
	requested, err := all.Only(only)
	if err != nil {
		return nil, nil, err
	}
	selected, err := requested.Exclude(exclude)
	if err != nil {
		return nil, nil, err
	}
	if len(selected) == 0 {
		return nil, nil, fmt.Errorf("no %s selected after applying --exclude", kind)
	}
	return selected, projectToolInvocationStatuses(all, requested, selected), nil
}
