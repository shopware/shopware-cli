package verifier

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type namedFixTool struct{ name string }

func (t namedFixTool) Name() string { return t.name }

func (namedFixTool) Fix(context.Context, ToolConfig, *ToolExecution) ToolSummary {
	return ToolSummary{}
}

func TestCookieListenerWiringIsAFollowUpOfRector(t *testing.T) {
	assert.Equal(t, ToolList[FixTool]{CookieListenerWiring{}}, FollowUpFixTools(ToolList[FixTool]{SymfonyXMLConverter{}, Rector{}}))
	assert.Empty(t, FollowUpFixTools(ToolList[FixTool]{SymfonyXMLConverter{}}))

	_, err := GetTools().Only(CookieListenerWiring{}.Name())
	assert.Error(t, err, "the wiring must not run in parallel with the symfony-xml fixer")
}

func TestRunFixToolsRunsFollowUpsAfterTheSelectedFixers(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "1")
	config := ToolConfig{RootDir: t.TempDir(), SourceDirectories: []string{t.TempDir()}}

	results := RunFixTools(t.Context(), ToolList[FixTool]{namedFixTool{name: Rector{}.Name()}}, nil, config, nil)

	require.Len(t, results, 2)
	assert.Equal(t, Rector{}.Name(), results[0].Name)
	assert.Equal(t, CookieListenerWiring{}.Name(), results[1].Name)
	assert.Equal(t, "no cookie provider was migrated", results[1].SkipReason)
}

func TestRunFixToolsWithoutFollowUps(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "1")

	results := RunFixTools(t.Context(), ToolList[FixTool]{namedFixTool{name: "eslint"}}, nil, ToolConfig{RootDir: t.TempDir()}, nil)

	require.Len(t, results, 1)
	assert.Equal(t, "eslint", results[0].Name)
}

func TestRunFixToolsSkipsFollowUpsWhenCancelled(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "1")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	results := RunFixTools(ctx, ToolList[FixTool]{namedFixTool{name: Rector{}.Name()}}, nil, ToolConfig{RootDir: t.TempDir()}, nil)

	require.Len(t, results, 1)
	assert.Equal(t, Rector{}.Name(), results[0].Name)
}
