package verifier

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
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
