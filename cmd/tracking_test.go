package cmd

import (
	"context"
	"errors"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tracking"
)

// useTrackingTestRoot swaps rootCmd for a minimal tree containing
// `project console` and `extension build-me` and captures tracked events.
func useTrackingTestRoot(t *testing.T, rootName string) *[]map[string]string {
	t.Helper()

	originalRoot, originalTrack := rootCmd, trackEvent
	t.Cleanup(func() {
		rootCmd, trackEvent = originalRoot, originalTrack
	})

	rootCmd = &cobra.Command{Use: rootName}
	projectCmd := &cobra.Command{Use: "project"}
	projectCmd.AddCommand(&cobra.Command{
		Use:                "console",
		DisableFlagParsing: true,
		RunE:               func(*cobra.Command, []string) error { return nil },
	})
	extensionCmd := &cobra.Command{Use: "extension"}
	extensionCmd.AddCommand(&cobra.Command{
		Use:  "build-me",
		RunE: func(*cobra.Command, []string) error { return nil },
	})
	rootCmd.AddCommand(projectCmd, extensionCmd)

	events := &[]map[string]string{}
	trackEvent = func(_ context.Context, event string, tags map[string]string) {
		assert.Equal(t, tracking.EventCommand, event)
		*events = append(*events, tags)
	}

	return events
}

func TestTrackCommandExecution_NormalizesNameAcrossBinaryNames(t *testing.T) {
	tests := []struct {
		name string
		argv []string
	}{
		{name: "regular binary", argv: []string{"shopware-cli", "project", "console", "cache:clear"}},
		{name: "swx alias", argv: []string{"/usr/local/bin/swx", "cache:clear"}},
		{name: "swx default command", argv: []string{"swx"}},
		{name: "windows alias", argv: []string{`C:\tools\swx.exe`, "cache:clear"}},
		{name: "renamed binary", argv: []string{"custom-cli", "project", "console", "cache:clear"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Execute sets rootCmd.Use to the invoked binary name before tracking.
			events := useTrackingTestRoot(t, commandNameFromArgs(tt.argv))

			trackCommandExecution(context.Background(), mapAliasArgs(tt.argv), time.Now(), nil)

			require.Len(t, *events, 1)
			assert.Equal(t, "project.console", (*events)[0][tracking.TagCommandName])
			assert.Equal(t, tracking.ResultSuccess, (*events)[0][tracking.TagResult])
		})
	}
}

func TestTrackCommandExecution_ReplacesHyphensAndSpaces(t *testing.T) {
	events := useTrackingTestRoot(t, "shopware-cli")

	trackCommandExecution(context.Background(), []string{"extension", "build-me", "./plugin"}, time.Now(), nil)

	require.Len(t, *events, 1)
	assert.Equal(t, "extension.build_me", (*events)[0][tracking.TagCommandName])
}

func TestTrackCommandExecution_Result(t *testing.T) {
	tests := []struct {
		name   string
		runErr error
		want   string
	}{
		{name: "success", runErr: nil, want: tracking.ResultSuccess},
		{name: "failure", runErr: errors.New("boom"), want: tracking.ResultFailure},
		{name: "cancelled", runErr: context.Canceled, want: tracking.ResultCancelled},
		{name: "wrapped cancelled", runErr: errors.Join(errors.New("stopped"), context.Canceled), want: tracking.ResultCancelled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := useTrackingTestRoot(t, "shopware-cli")

			trackCommandExecution(context.Background(), []string{"project", "console"}, time.Now(), tt.runErr)

			require.Len(t, *events, 1)
			assert.Equal(t, tt.want, (*events)[0][tracking.TagResult])
		})
	}
}

func TestTrackCommandExecution_Tags(t *testing.T) {
	events := useTrackingTestRoot(t, "shopware-cli")
	ctx := system.WithInteraction(context.Background(), true)

	trackCommandExecution(ctx, []string{"project", "console"}, time.Now().Add(-2*time.Second), nil)

	require.Len(t, *events, 1)
	tags := (*events)[0]
	assert.Equal(t, version, tags[tracking.TagCLIVersion])
	assert.Equal(t, runtime.GOOS, tags[tracking.TagOS])
	assert.Equal(t, "true", tags[tracking.TagIsTUI])

	durationMS, err := strconv.ParseInt(tags[tracking.TagDurationMS], 10, 64)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, durationMS, int64(2000))
}

func TestTrackCommandExecution_IsTUIFollowsInteraction(t *testing.T) {
	events := useTrackingTestRoot(t, "shopware-cli")
	ctx := system.WithInteraction(context.Background(), false)

	trackCommandExecution(ctx, []string{"project", "console"}, time.Now(), nil)

	require.Len(t, *events, 1)
	assert.Equal(t, "false", (*events)[0][tracking.TagIsTUI])
}

func TestTrackCommandExecution_SkipsNonRunnableInvocations(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "no arguments resolves to root", args: nil},
		{name: "root flag only", args: []string{"--help"}},
		{name: "group command without RunE", args: []string{"project"}},
		{name: "unknown command", args: []string{"does-not-exist"}},
		{name: "unknown subcommand", args: []string{"project", "does-not-exist"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := useTrackingTestRoot(t, "shopware-cli")

			trackCommandExecution(context.Background(), tt.args, time.Now(), nil)

			assert.Empty(t, *events)
		})
	}
}

func TestTrackCommandExecution_TracksAfterContextCancelled(t *testing.T) {
	events := useTrackingTestRoot(t, "shopware-cli")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var trackCtxErr error
	originalTrack := trackEvent
	trackEvent = func(trackCtx context.Context, event string, tags map[string]string) {
		trackCtxErr = trackCtx.Err()
		originalTrack(trackCtx, event, tags)
	}

	trackCommandExecution(ctx, []string{"project", "console"}, time.Now(), ctx.Err())

	require.Len(t, *events, 1)
	assert.Equal(t, tracking.ResultCancelled, (*events)[0][tracking.TagResult])
	assert.NoError(t, trackCtxErr, "tracking must get a live context even after the command context was cancelled")
}
