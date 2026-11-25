package verifier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/tracking"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/logging"
)

func TestResolveQATimeout(t *testing.T) {
	tests := []struct {
		name, flag, env string
		set             bool
		want            time.Duration
		invalid         bool
	}{
		{name: "default", want: 0},
		{name: "environment", env: "2m", want: 2 * time.Minute},
		{name: "compound flag", flag: "1h30m", env: "0", set: true, want: 90 * time.Minute},
		{name: "environment zero", env: "0"},
		{name: "invalid flag", flag: "bad", set: true, invalid: true},
		{name: "flag precedence", flag: "0", env: "invalid", set: true},
		{name: "explicit default overrides environment", flag: DefaultQATimeout.String(), env: "2m", set: true, want: DefaultQATimeout},
		{name: "negative", env: "-1s", invalid: true},
		{name: "invalid", env: "no", invalid: true},
		{name: "overflow", env: "999999999999999999999h", invalid: true},
		{name: "empty explicit", set: true, invalid: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveQATimeout(tt.flag, tt.set, tt.env)
			if tt.invalid {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

type finishSignalWriter struct {
	mu    sync.Mutex
	armed bool
	done  chan struct{}
	once  sync.Once
}

// Write signals the first progress write after the completed tool is armed.
// This coordinates completion without depending on progress wording.
func (w *finishSignalWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.armed {
		w.once.Do(func() { close(w.done) })
	}
	return len(p), nil
}

func TestQATimeoutKeepsCompletedToolResults(t *testing.T) {
	parent, stop := context.WithTimeout(t.Context(), 5*time.Second)
	defer stop()
	ctx, cancel := context.WithCancelCause(parent)
	defer cancel(nil)
	finished := &finishSignalWriter{done: make(chan struct{})}
	running := make(chan struct{})
	done := make(chan []validation.ToolReport, 1)
	go func() {
		done <- RunTools(ctx, ToolList[Tool]{testTool{name: "completed"}, testTool{name: "running"}}, nil, ToolConfig{}, &Progress{output: finished}, func(tool Tool, execution *ToolExecution) ToolSummary {
			if tool.Name() == "completed" {
				select {
				case <-running:
				case <-ctx.Done():
					return ToolSummary{}
				}
				execution.AddFindings(validation.CheckResult{Message: "finding"})
				finished.mu.Lock()
				finished.armed = true
				finished.mu.Unlock()
				return ToolSummary{}
			}
			close(running)
			select {
			case <-finished.done:
				cancel(ErrQATimeout)
			case <-ctx.Done():
			}
			return ToolSummary{}
		})
	}()
	var reports []validation.ToolReport
	select {
	case reports = <-done:
	case <-parent.Done():
		t.Fatal("tools did not complete")
	}

	assert.True(t, reports[0].HasFindings)
	assert.Empty(t, reports[0].Errors)
	if assert.NotEmpty(t, reports[1].Errors) {
		assert.ErrorIs(t, reports[1].Errors[0], ErrQATimeout)
	}
	assert.Equal(t, "running", tracking.CommandTags()[tracking.TagQATimeoutTools])
}

func TestQAPhaseErrorDistinguishesParentCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	assert.ErrorIs(t, QAPhaseError(ctx, QAPhaseToolSetup, context.Canceled), context.Canceled)
	timeout, stop := context.WithTimeoutCause(t.Context(), time.Nanosecond, ErrQATimeout)
	defer stop()
	<-timeout.Done()
	err := QAPhaseError(timeout, QAPhaseToolSetup, timeout.Err())
	assert.ErrorIs(t, err, ErrQATimeout)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestQAPhaseErrorTelemetryPhase(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(ErrQATimeout)

	for _, tt := range []struct {
		phase string
		want  string
		label string
	}{
		{phase: QAPhaseExtensionDiscovery, want: "extension_discovery", label: "extension discovery"},
		{phase: QAPhaseProjectDiscovery, want: "project_discovery", label: "project discovery"},
		{phase: QAPhaseConfigurationDiscovery, want: "configuration_discovery", label: "configuration discovery"},
		{phase: QAPhaseArchiveExtraction, want: "archive_extraction", label: "archive extraction"},
		{phase: QAPhaseToolSetup, want: "tool_setup", label: "tool setup"},
		{phase: QAPhaseSetupAndDiscovery, want: "tool_setup_and_configuration_discovery", label: "tool setup and configuration discovery"},
		{phase: QAPhaseExecution, want: "execution", label: "execution"},
	} {
		t.Run(tt.phase, func(t *testing.T) {
			err := QAPhaseError(ctx, tt.phase, nil)
			require.Error(t, err)
			assert.Equal(t, tt.want, tt.phase)
			assert.Contains(t, err.Error(), tt.label)
			assert.Equal(t, tt.want, tracking.CommandTags()[tracking.TagQATimeoutPhase])
		})
	}
}

func TestQAPhaseErrorPreservesSetupErrors(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(ErrQATimeout)

	t.Run("wrapped context error", func(t *testing.T) {
		setupErr := fmt.Errorf("cannot read configuration: %w", context.DeadlineExceeded)
		err := QAPhaseError(ctx, QAPhaseToolSetup, setupErr)

		assert.ErrorIs(t, err, ErrQATimeout)
		assert.ErrorIs(t, err, setupErr)
		assert.Contains(t, err.Error(), "cannot read configuration")
	})

	t.Run("joined independent error", func(t *testing.T) {
		independentErr := errors.New("configuration unavailable")
		setupErr := errors.Join(context.Canceled, independentErr)
		err := QAPhaseError(ctx, QAPhaseToolSetup, setupErr)

		assert.ErrorIs(t, err, ErrQATimeout)
		assert.ErrorIs(t, err, context.Canceled)
		assert.ErrorIs(t, err, independentErr)
		assert.Contains(t, err.Error(), "configuration unavailable")
	})
}

func TestWithQATimeout(t *testing.T) {
	t.Run("default leaves parent unchanged", func(t *testing.T) {
		t.Setenv(QATimeoutEnv, "")
		parent := t.Context()
		ctx, cancel, err := WithQATimeout(parent, "", false)
		require.NoError(t, err)
		assert.Same(t, parent, ctx)
		cancel()
		assert.NoError(t, parent.Err())
		assert.Equal(t, "0", tracking.CommandTags()[tracking.TagQATimeoutMS])
	})

	t.Run("own timeout overrides invalid environment", func(t *testing.T) {
		t.Setenv(QATimeoutEnv, "invalid")
		ctx, cancel, err := WithQATimeout(t.Context(), "1ms", true)
		require.NoError(t, err)
		defer cancel()

		<-ctx.Done()
		err = QAPhaseError(ctx, QAPhaseToolSetup, ctx.Err())
		assert.ErrorIs(t, err, ErrQATimeout)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Contains(t, err.Error(), "1ms")
	})

	t.Run("parent cancellation", func(t *testing.T) {
		parent, stop := context.WithCancel(t.Context())
		stop()
		ctx, cancel, err := WithQATimeout(parent, "1h", true)
		require.NoError(t, err)
		defer cancel()
		err = QAPhaseError(ctx, QAPhaseToolSetup, ctx.Err())
		assert.ErrorIs(t, err, context.Canceled)
		assert.NotErrorIs(t, err, ErrQATimeout)
	})

	t.Run("parent deadline", func(t *testing.T) {
		parent, stop := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
		defer stop()
		ctx, cancel, err := WithQATimeout(parent, "1h", true)
		require.NoError(t, err)
		defer cancel()
		err = QAPhaseError(ctx, QAPhaseToolSetup, ctx.Err())
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.NotErrorIs(t, err, ErrQATimeout)
	})

	t.Run("zero overrides invalid environment", func(t *testing.T) {
		t.Setenv(QATimeoutEnv, "invalid")
		parent := t.Context()
		ctx, cancel, err := WithQATimeout(parent, "0", true)
		require.NoError(t, err)
		defer cancel()
		assert.Same(t, parent, ctx)
	})

	t.Run("invalid environment", func(t *testing.T) {
		t.Setenv(QATimeoutEnv, "invalid")
		_, _, err := WithQATimeout(t.Context(), "", false)
		assert.Error(t, err)
	})
}

func TestEnsureQATimeoutResult(t *testing.T) {
	t.Run("records timeout and preserves completed results", func(t *testing.T) {
		ctx, cancel, err := WithQATimeout(t.Context(), "1ms", true)
		require.NoError(t, err)
		<-ctx.Done()
		reports := []validation.ToolReport{{ToolResult: validation.ToolResult{Name: "completed"}, Findings: []validation.CheckResult{{Message: "retained"}}}}
		cancel()
		reports = EnsureQATimeoutResult(ctx, reports)
		require.Len(t, reports, 2)
		assert.Equal(t, "retained", reports[0].Findings[0].Message)
		assert.ErrorIs(t, reports[1].Errors[0], ErrQATimeout)
		assert.Len(t, EnsureQATimeoutResult(ctx, reports), 2)
	})

	t.Run("stops timer after successful execution", func(t *testing.T) {
		ctx, cancel, err := WithQATimeout(t.Context(), "1h", true)
		require.NoError(t, err)
		cancel()
		assert.Empty(t, EnsureQATimeoutResult(ctx, nil))
		assert.ErrorIs(t, context.Cause(ctx), context.Canceled)
	})
}

func TestQATimeoutReportsInterruptedToolAndPartialChanges(t *testing.T) {
	root := t.TempDir()
	changed := filepath.Join(root, "changed.php")
	require.NoError(t, os.WriteFile(changed, []byte("before"), 0o600))
	old := os.Stdout
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = writer
	t.Cleanup(func() {
		os.Stdout = old
		_ = writer.Close()
		_ = reader.Close()
	})
	var writeErr error
	ctx, cancel, err := WithQATimeout(t.Context(), "500ms", true)
	require.NoError(t, err)
	defer cancel()
	reports := RunTools(ctx, ToolList[Tool]{testTool{name: "running"}}, nil, ToolConfig{RootDir: root}, nil, func(_ Tool, execution *ToolExecution) ToolSummary {
		writeErr = os.WriteFile(changed, []byte("edited"), 0o600)
		if writeErr != nil {
			return ToolSummary{}
		}
		execution.AddChangedFiles(changed)
		execution.AddFindings(validation.CheckResult{Message: "retained finding"})
		subprocess := execution.Run(OperationFix, CommandContext(ctx, "sh", "-c", "exec sleep 30"))
		if subprocess.Err != nil {
			execution.Fail(subprocess.ToolError(subprocess.Err))
		}
		return ToolSummary{}
	})
	cancel()

	reports = EnsureQATimeoutResult(ctx, reports)
	err = validation.ReportTools(reports, "json", "Fixers")
	require.NoError(t, writer.Close())
	data, readErr := io.ReadAll(reader)
	require.NoError(t, readErr)
	require.NoError(t, writeErr)
	content, readErr := os.ReadFile(changed)
	require.NoError(t, readErr)
	assert.Equal(t, "edited", string(content))
	assert.Error(t, err)
	var reportErr *validation.ToolReportError
	assert.ErrorAs(t, err, &reportErr)
	var silent logging.SilenceError
	assert.ErrorAs(t, err, &silent)
	var report struct {
		Results []validation.CheckResult `json:"results"`
		Tools   []struct {
			Name         string   `json:"name"`
			ChangedFiles []string `json:"changed_files"`
			Errors       []struct {
				Message string `json:"message"`
			} `json:"errors"`
		} `json:"tools"`
	}
	require.NoError(t, json.Unmarshal(data, &report))
	require.Len(t, report.Results, 1)
	assert.Equal(t, "retained finding", report.Results[0].Message)
	require.Len(t, report.Tools, 1)
	assert.Equal(t, "running", report.Tools[0].Name)
	assert.Equal(t, []string{"changed.php"}, report.Tools[0].ChangedFiles)
	require.NotEmpty(t, report.Tools[0].Errors)
	assert.Contains(t, report.Tools[0].Errors[0].Message, "timed out")
	assert.Len(t, report.Tools[0].Errors, 2)
}
