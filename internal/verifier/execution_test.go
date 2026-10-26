package verifier

import (
	"errors"
	"os/exec"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/validation"
)

func TestToolExecutionCollectsConcurrentFailures(t *testing.T) {
	execution := NewToolExecution(func([]validation.CheckResult) bool { return true })
	failure := errors.New("cannot run tool")
	var group sync.WaitGroup
	for range 3 {
		group.Go(func() {
			execution.Fail(&ToolError{Err: failure})
		})
	}
	group.Wait()
	require.Len(t, execution.snapshot().Errors, 3)
}

func TestToolExecutionRunDeliversDiagnosticsWithoutReportingFailure(t *testing.T) {
	execution := NewToolExecution(func([]validation.CheckResult) bool { return false })
	delivered := false
	execution.onSubprocess = func(subprocess SubprocessResult) {
		delivered = true
		assert.Equal(t, "output", subprocess.Stdout)
	}
	subprocess := execution.Run(exec.CommandContext(t.Context(), "/bin/sh", "-c", "printf output; exit 1"))
	assert.True(t, delivered)
	require.Error(t, subprocess.Err)
	assert.Empty(t, execution.snapshot().Errors)
	cause := errors.New("cannot parse output")
	execution.Fail(subprocess.ToolError(cause))
	assert.ErrorIs(t, execution.snapshot().Errors[0], cause)
	execution.onSubprocess = nil
	subprocess = execution.Run(exec.CommandContext(t.Context(), "/missing-executable"))
	require.Error(t, subprocess.Err)
	assert.Len(t, execution.snapshot().Errors, 1, "Run must leave failure classification to the tool")
}

func TestToolExecutionRunProcessState(t *testing.T) {
	for _, tt := range []struct {
		name   string
		script string
		code   int
	}{
		{"success", "exit 0", 0},
		{"nonzero exit", "exit 2", 2},
		{"signal", "printf partial; kill -TERM $$", -1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			execution := NewToolExecution(func([]validation.CheckResult) bool { return false })
			result := execution.Run(exec.CommandContext(t.Context(), "/bin/sh", "-c", tt.script))
			require.NotNil(t, result.ProcessState)
			assert.Equal(t, tt.code, result.ProcessState.ExitCode())
			if tt.code == -1 {
				assert.Equal(t, "partial", result.Stdout)
			}
		})
	}
	execution := NewToolExecution(func([]validation.CheckResult) bool { return false })
	result := execution.Run(exec.CommandContext(t.Context(), "/missing-executable"))
	require.Error(t, result.Err)
	assert.Nil(t, result.ProcessState)
}

func runCheckTool(t *testing.T, tool CheckTool, config ToolConfig) validation.ToolReport {
	t.Helper()
	t.Setenv("DO_NOT_TRACK", "1")
	return RunTools(t.Context(), ToolList[CheckTool]{tool}, nil, config, nil, func(tool CheckTool, execution *ToolExecution) ToolSummary {
		return tool.Check(t.Context(), config, execution)
	})[0]
}

func runFixTool(t *testing.T, tool FixTool, config ToolConfig) validation.ToolReport {
	t.Helper()
	t.Setenv("DO_NOT_TRACK", "1")
	return RunTools(t.Context(), ToolList[FixTool]{tool}, nil, config, nil, func(tool FixTool, execution *ToolExecution) ToolSummary {
		return tool.Fix(t.Context(), config, execution)
	})[0]
}

func runFormatTool(t *testing.T, tool FormatTool, config ToolConfig, dryRun bool) validation.ToolReport {
	t.Helper()
	t.Setenv("DO_NOT_TRACK", "1")
	return RunTools(t.Context(), ToolList[FormatTool]{tool}, nil, config, nil, func(tool FormatTool, execution *ToolExecution) ToolSummary {
		return tool.Format(t.Context(), config, execution, dryRun)
	})[0]
}
