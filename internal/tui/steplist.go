package tui

import (
	"strings"
)

// StepState is the display state of one StepItem.
type StepState int

const (
	StepStatePending StepState = iota
	StepStateActive
	StepStateDone
)

// StepItem is one row in a StepList.
type StepItem struct {
	Label string
	State StepState
	// Indicator overrides the state indicator, e.g. a spinner frame for the
	// active step.
	Indicator string
}

// StepListOptions configure a StepList.
type StepListOptions struct {
	Steps []StepItem
}

// StepList renders a vertical checklist of steps with per-state indicators.
type StepList struct {
	opts StepListOptions
}

// NewStepList creates a step list.
func NewStepList(opts StepListOptions) StepList {
	return StepList{opts: opts}
}

// Render implements the component contract.
func (l StepList) Render() string {
	var b strings.Builder
	for _, step := range l.opts.Steps {
		b.WriteString(l.renderStep(step))
	}
	return b.String()
}

func (l StepList) renderStep(step StepItem) string {
	indicator := step.Indicator
	if indicator == "" {
		switch step.State {
		case StepStateDone:
			indicator = Checkmark
		case StepStatePending:
			indicator = DimStyle.Render("·")
		case StepStateActive:
			indicator = ""
		}
	}
	return fixedIndicator(indicator) + step.Label + "\n"
}
