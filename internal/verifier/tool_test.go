package verifier

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

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
