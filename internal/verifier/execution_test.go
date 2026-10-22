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

func TestToolExecutionCapturesIndependentSubprocesses(t *testing.T) {
	execution := NewToolExecution(func([]validation.CheckResult) bool { return true })
	var group sync.WaitGroup
	for range 3 {
		group.Go(func() {
			cmd := exec.CommandContext(t.Context(), "/bin/sh", "-c", "printf partial; printf diagnostic >&2; exit 2")
			cmd.Dir = t.TempDir()
			subprocess := execution.Run(cmd)
			assert.Equal(t, "partial", subprocess.Stdout)
			assert.Equal(t, "diagnostic", subprocess.Stderr)
			assert.Equal(t, cmd.Dir, subprocess.Dir)
			assert.Equal(t, cmd.Args, subprocess.Command)
			execution.Fail(subprocess.ToolError(subprocess.Err))
			execution.AddFindings(validation.CheckResult{Identifier: "kept"})
		})
	}
	group.Wait()
	result := execution.Result()
	assert.True(t, result.HasFindings)
	require.Len(t, result.Errors, 3)
	var exitError *exec.ExitError
	assert.ErrorAs(t, result.Errors[0], &exitError)
	result.Errors[0].Command[0] = "changed"
	assert.Equal(t, "/bin/sh", execution.Result().Errors[0].Command[0])
}

func TestToolExecutionRunDeliversDiagnosticsWithoutReportingFailure(t *testing.T) {
	execution := NewToolExecution(func([]validation.CheckResult) bool { return false })
	var telemetry map[string]string
	execution.telemetry = func(tags map[string]string) { telemetry = tags }
	delivered := false
	execution.onSubprocess = func(subprocess SubprocessResult) {
		delivered = true
		assert.Equal(t, "output", subprocess.Stdout)
	}
	subprocess := execution.Run(exec.CommandContext(t.Context(), "/bin/sh", "-c", "printf output; exit 1"))
	assert.True(t, delivered)
	require.Error(t, subprocess.Err)
	assert.Equal(t, map[string]string{"started": "true", "exit_code": "1", "stdout_bytes": "6", "stderr_bytes": "0"}, telemetry)
	assert.Empty(t, execution.Result().Errors)
	cause := errors.New("cannot parse output")
	execution.Fail(subprocess.ToolError(cause))
	assert.ErrorIs(t, execution.Result().Errors[0], cause)
	execution.onSubprocess = nil
	subprocess = execution.Run(exec.CommandContext(t.Context(), "/missing-executable"))
	require.Error(t, subprocess.Err)
	assert.Equal(t, map[string]string{"started": "false", "stdout_bytes": "0", "stderr_bytes": "0"}, telemetry)
	assert.Len(t, execution.Result().Errors, 1, "Run must leave failure classification to the tool")
}

func runCheckTool(t *testing.T, tool CheckTool, config ToolConfig) validation.ToolReport {
	t.Helper()
	t.Setenv("DO_NOT_TRACK", "1")
	return RunTools(t.Context(), ToolList[CheckTool]{tool}, nil, config, func(tool CheckTool, execution *ToolExecution) ToolResult {
		return tool.Check(t.Context(), config, execution)
	})[0]
}

func runFixTool(t *testing.T, tool FixTool, config ToolConfig) validation.ToolReport {
	t.Helper()
	t.Setenv("DO_NOT_TRACK", "1")
	return RunTools(t.Context(), ToolList[FixTool]{tool}, nil, config, func(tool FixTool, execution *ToolExecution) ToolResult {
		return tool.Fix(t.Context(), config, execution)
	})[0]
}

func runFormatTool(t *testing.T, tool FormatTool, config ToolConfig, dryRun bool) validation.ToolReport {
	t.Helper()
	t.Setenv("DO_NOT_TRACK", "1")
	return RunTools(t.Context(), ToolList[FormatTool]{tool}, nil, config, func(tool FormatTool, execution *ToolExecution) ToolResult {
		return tool.Format(t.Context(), config, execution, dryRun)
	})[0]
}
