package verifier

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/logging"
)

type testTool struct{ name string }

func (t testTool) Name() string                                                     { return t.name }
func (t testTool) Check(ctx context.Context, check *Check, config ToolConfig) error { return nil }
func (t testTool) Fix(ctx context.Context, config ToolConfig) error                 { return nil }
func (t testTool) Format(ctx context.Context, config ToolConfig, dryRun bool) error { return nil }

func toolNames[T Tool](list ToolList[T]) []string {
	out := make([]string, 0, len(list))
	for _, t := range list {
		out = append(out, t.Name())
	}
	return out
}

func TestToolsByCapability(t *testing.T) {
	assert.ElementsMatch(t, []string{"eslint", "phpstan", "storefront-twig", "stylelint", "builtin"}, toolNames(GetToolsOf[CheckTool]()))
	assert.ElementsMatch(t, []string{"eslint", "rector", "stylelint", "symfony-xml"}, toolNames(GetToolsOf[FixTool]()))
	assert.ElementsMatch(t, []string{"php-cs-fixer", "prettier"}, toolNames(GetToolsOf[FormatTool]()))
}

func TestOnly_DeduplicatesAndPreservesOrder(t *testing.T) {
	t.Parallel()
	base := ToolList[testTool]{testTool{"phpstan"}, testTool{"eslint"}, testTool{"builtin"}}
	res, err := base.Only("eslint, phpstan,eslint")
	assert.NoError(t, err)
	assert.Equal(t, []string{"eslint", "phpstan"}, toolNames(res))
	res, err = base.Only("eslint,eslint,unknown")
	assert.ErrorContains(t, err, `tool with name "unknown" not found`)
	assert.Nil(t, res)
}

func TestExclude_EmptyString_NoChange(t *testing.T) {
	t.Parallel()
	base := ToolList[testTool]{testTool{"phpstan"}, testTool{"eslint"}, testTool{"builtin"}}
	res, err := base.Exclude("")
	assert.NoError(t, err)
	assert.Equal(t, toolNames(base), toolNames(res))
}

func TestExclude_SingleTool(t *testing.T) {
	t.Parallel()
	base := ToolList[testTool]{testTool{"phpstan"}, testTool{"eslint"}, testTool{"builtin"}}
	res, err := base.Exclude("eslint")
	assert.NoError(t, err)
	assert.Equal(t, []string{"phpstan", "builtin"}, toolNames(res))
}

func TestExclude_MultipleTools(t *testing.T) {
	t.Parallel()
	base := ToolList[testTool]{testTool{"phpstan"}, testTool{"eslint"}, testTool{"builtin"}, testTool{"stylelint"}}
	res, err := base.Exclude("eslint, stylelint")
	assert.NoError(t, err)
	assert.Equal(t, []string{"phpstan", "builtin"}, toolNames(res))
}

func TestExclude_AllTools_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	base := ToolList[testTool]{testTool{"phpstan"}, testTool{"eslint"}}
	res, err := base.Exclude("phpstan,eslint")
	assert.NoError(t, err)
	assert.Empty(t, res)
}

func TestExclude_UnknownTool_Error(t *testing.T) {
	t.Parallel()
	base := ToolList[testTool]{testTool{"phpstan"}, testTool{"eslint"}}
	res, err := base.Exclude("rector")
	assert.Error(t, err)
	assert.Nil(t, res)
}

func TestExclude_TrimsAndIgnoresDuplicates(t *testing.T) {
	t.Parallel()
	base := ToolList[testTool]{testTool{"phpstan"}, testTool{"eslint"}, testTool{"builtin"}}
	res, err := base.Exclude(" , eslint , eslint ,  \teslint\t , ")
	assert.NoError(t, err)
	assert.Equal(t, []string{"phpstan", "builtin"}, toolNames(res))
}

func TestCompleteNames(t *testing.T) {
	tools := ToolList[Tool]{testTool{name: "eslint"}, testTool{name: "phpstan"}, testTool{name: "builtin"}}

	assert.ElementsMatch(t, []string{"eslint", "phpstan", "builtin"}, tools.CompleteNames(""))
	assert.ElementsMatch(t, []string{"eslint", "phpstan", "builtin"}, tools.CompleteNames("php"))
	assert.ElementsMatch(t, []string{"phpstan,eslint", "phpstan,builtin"}, tools.CompleteNames("phpstan,"))
	assert.ElementsMatch(t, []string{"phpstan,eslint", "phpstan,builtin"}, tools.CompleteNames("phpstan,b"))
	assert.ElementsMatch(t, []string{"sw-cli,eslint", "sw-cli,phpstan"}, tools.CompleteNames("sw-cli,"))
	assert.Empty(t, tools.CompleteNames("eslint,phpstan,builtin,"))
}

func TestOnly_LegacyBuiltinAlias(t *testing.T) {
	base := ToolList[testTool]{testTool{"phpstan"}, testTool{"builtin"}}
	res, err := base.Only("sw-cli")
	assert.NoError(t, err)
	assert.Equal(t, []string{"builtin"}, toolNames(res))
}

func TestExclude_LegacyBuiltinAlias(t *testing.T) {
	base := ToolList[testTool]{testTool{"phpstan"}, testTool{"builtin"}}
	res, err := base.Exclude("sw-cli")
	assert.NoError(t, err)
	assert.Equal(t, []string{"phpstan"}, toolNames(res))
}

func TestWarnOnDeprecatedToolName(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	ctx := logging.WithLogger(t.Context(), zap.New(core).Sugar())

	WarnOnDeprecatedToolName(ctx, "phpstan,builtin", "sw-cli")

	if assert.Len(t, logs.All(), 1) {
		assert.Contains(t, logs.All()[0].Message, "sw-cli")
		assert.Contains(t, logs.All()[0].Message, "builtin")
		assert.Equal(t, "warn", logs.All()[0].Level.String())
	}
}

func TestSelectTools(t *testing.T) {
	all := ToolList[Tool]{testTool{name: "alpha"}, testTool{name: "beta"}, testTool{name: "gamma"}}

	t.Run("selects all by default", func(t *testing.T) {
		selected, statuses, err := SelectTools(all, "", "", "formatters")
		require.NoError(t, err)
		assert.Equal(t, all, selected)
		assert.Equal(t, []validation.ToolInvocationStatus{
			{Name: "alpha", Status: "invoked"},
			{Name: "beta", Status: "invoked"},
			{Name: "gamma", Status: "invoked"},
		}, statuses)
	})

	t.Run("applies only and exclude and sorts statuses", func(t *testing.T) {
		selected, statuses, err := SelectTools(all, "gamma,beta", "beta", "formatters")
		require.NoError(t, err)
		assert.Equal(t, ToolList[Tool]{testTool{name: "gamma"}}, selected)
		assert.Equal(t, []validation.ToolInvocationStatus{
			{Name: "alpha", Status: "skipped", Reason: "not selected by --only"},
			{Name: "beta", Status: "skipped", Reason: "excluded by --exclude"},
			{Name: "gamma", Status: "invoked"},
		}, statuses)
	})

	t.Run("rejects excluding every selected tool", func(t *testing.T) {
		_, statuses, err := SelectTools(all, "beta", "beta", "formatters")
		require.ErrorContains(t, err, "no formatters selected after applying --exclude")
		assert.Nil(t, statuses)
	})

	t.Run("rejects excluding a tool outside the only set", func(t *testing.T) {
		_, _, err := SelectTools(all, "alpha", "beta", "formatters")
		require.ErrorContains(t, err, "not found in the selected tools")
	})
}

func TestToolInvocationStatuses(t *testing.T) {
	all := ToolList[Tool]{testTool{name: "alpha"}, testTool{name: "beta"}, testTool{name: "gamma"}}
	requested := ToolList[Tool]{testTool{name: "gamma"}, testTool{name: "beta"}}
	selected := ToolList[Tool]{testTool{name: "gamma"}}

	assert.Equal(t, []validation.ToolInvocationStatus{
		{Name: "alpha", Status: "skipped", Reason: "not selected by --only"},
		{Name: "beta", Status: "skipped", Reason: "excluded by --exclude"},
		{Name: "gamma", Status: "invoked"},
	}, ToolInvocationStatuses(all, requested, selected))
}

func TestCommandOutputOrError(t *testing.T) {
	tests := []struct {
		name   string
		stderr string
		stdout []byte
		err    error
		want   string
	}{
		{
			name:   "prefers stderr",
			stderr: " stderr ",
			stdout: []byte("stdout"),
			err:    errors.New("process failed"),
			want:   "stderr",
		},
		{
			name:   "falls back to stdout",
			stdout: []byte(" stdout "),
			err:    errors.New("process failed"),
			want:   "stdout",
		},
		{
			name: "falls back to process error",
			err:  errors.New("process failed"),
			want: "process failed",
		},
		{
			name: "empty when there is no output or error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, commandOutputOrError(tt.stderr, tt.stdout, tt.err))
		})
	}
}
