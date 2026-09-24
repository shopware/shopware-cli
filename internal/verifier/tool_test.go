package verifier

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/validation"
)

type testTool struct{ name string }

func (t testTool) Name() string                                                     { return t.name }
func (t testTool) Check(ctx context.Context, check *Check, config ToolConfig) error { return nil }
func (t testTool) Fix(ctx context.Context, config ToolConfig) error                 { return nil }
func (t testTool) Format(ctx context.Context, config ToolConfig, dryRun bool) error { return nil }

func toolNames(list ToolList) []string {
	out := make([]string, 0, len(list))
	for _, t := range list {
		out = append(out, t.Name())
	}
	return out
}

func TestExclude_EmptyString_NoChange(t *testing.T) {
	t.Parallel()
	base := ToolList{testTool{"phpstan"}, testTool{"eslint"}, testTool{"sw-cli"}}
	res, err := base.Exclude("")
	assert.NoError(t, err)
	assert.Equal(t, toolNames(base), toolNames(res))
}

func TestExclude_SingleTool(t *testing.T) {
	t.Parallel()
	base := ToolList{testTool{"phpstan"}, testTool{"eslint"}, testTool{"sw-cli"}}
	res, err := base.Exclude("eslint")
	assert.NoError(t, err)
	assert.Equal(t, []string{"phpstan", "sw-cli"}, toolNames(res))
}

func TestExclude_MultipleTools(t *testing.T) {
	t.Parallel()
	base := ToolList{testTool{"phpstan"}, testTool{"eslint"}, testTool{"sw-cli"}, testTool{"stylelint"}}
	res, err := base.Exclude("eslint, stylelint")
	assert.NoError(t, err)
	assert.Equal(t, []string{"phpstan", "sw-cli"}, toolNames(res))
}

func TestExclude_AllTools_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	base := ToolList{testTool{"phpstan"}, testTool{"eslint"}}
	res, err := base.Exclude("phpstan,eslint")
	assert.NoError(t, err)
	assert.Empty(t, res)
}

func TestExclude_UnknownTool_Error(t *testing.T) {
	t.Parallel()
	base := ToolList{testTool{"phpstan"}, testTool{"eslint"}}
	res, err := base.Exclude("rector")
	assert.Error(t, err)
	assert.Nil(t, res)
}

func TestExclude_TrimsAndIgnoresDuplicates(t *testing.T) {
	t.Parallel()
	base := ToolList{testTool{"phpstan"}, testTool{"eslint"}, testTool{"sw-cli"}}
	res, err := base.Exclude(" eslint , eslint ,  \teslint\t ")
	assert.NoError(t, err)
	assert.Equal(t, []string{"phpstan", "sw-cli"}, toolNames(res))
}

type recordingTool struct {
	name string
	run  *validation.ToolRun
	err  error
}

func (r recordingTool) Name() string { return r.name }
func (r recordingTool) Check(ctx context.Context, check *Check, config ToolConfig) error {
	if r.run != nil {
		check.RecordToolRun(*r.run)
	}
	return r.err
}
func (r recordingTool) Fix(ctx context.Context, config ToolConfig) error                 { return nil }
func (r recordingTool) Format(ctx context.Context, config ToolConfig, dryRun bool) error { return nil }

func TestRunChecksRecordsTargetAndDefaultRuns(t *testing.T) {
	t.Parallel()
	tools := ToolList{
		recordingTool{name: "eslint", run: &validation.ToolRun{Name: "eslint", Status: validation.ToolRunRan, Baseline: "6.7.0.0"}},
		recordingTool{name: "stylelint"},
	}
	check := NewCheck()
	cfg := ToolConfig{Target: validation.Target{Version: "6.7.0.0", Source: validation.TargetSourceConstraint}}

	require.NoError(t, tools.RunChecks(t.Context(), check, cfg))

	require.NotNil(t, check.GetTarget())
	assert.Equal(t, "6.7.0.0", check.GetTarget().Version)
	assert.Equal(t, []validation.ToolRun{
		{Name: "eslint", Status: validation.ToolRunRan, Baseline: "6.7.0.0"},
		{Name: "stylelint", Status: validation.ToolRunRan},
	}, check.GetToolRuns())
}

func TestRunChecksReturnsToolError(t *testing.T) {
	t.Parallel()
	tools := ToolList{recordingTool{name: "broken", err: errors.New("boom")}}

	err := tools.RunChecks(t.Context(), NewCheck(), ToolConfig{})

	assert.EqualError(t, err, "boom")
}
