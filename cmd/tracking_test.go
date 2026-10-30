package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tracking"
	"github.com/shopware/shopware-cli/internal/tui"
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
	}, &cobra.Command{
		Use:  "create",
		RunE: func(*cobra.Command, []string) error { return nil },
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

			trackCommandExecution(context.Background(), mapAliasArgs(tt.argv), time.Now(), nil, "")

			require.Len(t, *events, 1)
			assert.Equal(t, "project.console", (*events)[0][tracking.TagCommandName])
			assert.Equal(t, tracking.ResultSuccess, (*events)[0][tracking.TagResult])
		})
	}
}

func TestTrackCommandExecution_ReplacesHyphensAndSpaces(t *testing.T) {
	events := useTrackingTestRoot(t, "shopware-cli")

	trackCommandExecution(context.Background(), []string{"extension", "build-me", "./plugin"}, time.Now(), nil, "")

	require.Len(t, *events, 1)
	assert.Equal(t, "extension.build_me", (*events)[0][tracking.TagCommandName])
}

func TestTrackCommandExecution_ExtensionAddGenerator(t *testing.T) {
	originalRoot, originalTrack := rootCmd, trackEvent
	t.Cleanup(func() {
		rootCmd, trackEvent = originalRoot, originalTrack
	})

	rootCmd = &cobra.Command{Use: "shopware-cli"}
	addCmd := &cobra.Command{Use: "add"}
	addCmd.AddCommand(&cobra.Command{
		Use:  "admin-module",
		RunE: func(*cobra.Command, []string) error { return nil },
	})
	extensionCmd := &cobra.Command{Use: "extension"}
	extensionCmd.AddCommand(addCmd)
	rootCmd.AddCommand(extensionCmd)

	var events []map[string]string
	trackEvent = func(_ context.Context, event string, tags map[string]string) {
		assert.Equal(t, tracking.EventCommand, event)
		events = append(events, tags)
	}

	trackCommandExecution(context.Background(), []string{"extension", "add", "admin-module"}, time.Now(), nil, "")

	require.Len(t, events, 1)
	assert.Equal(t, "extension.add.admin_module", events[0][tracking.TagCommandName])
}

func TestTrackCommandExecution_Result(t *testing.T) {
	tests := []struct {
		name        string
		interrupted bool
		runErr      error
		want        string
	}{
		{name: "success", runErr: nil, want: tracking.ResultSuccess},
		{name: "failure", runErr: errors.New("boom"), want: tracking.ResultFailure},
		{name: "cancelled", runErr: context.Canceled, want: tracking.ResultCancelled},
		{name: "wrapped cancelled", runErr: errors.Join(errors.New("stopped"), context.Canceled), want: tracking.ResultCancelled},
		{name: "interrupted run with a child exit error", interrupted: true, runErr: errors.New("exit status 130"), want: tracking.ResultCancelled},
		{name: "interrupted run that still succeeded", interrupted: true, runErr: nil, want: tracking.ResultSuccess},
		{name: "prompt aborted with ctrl+c", runErr: fmt.Errorf("cannot create project: %w", huh.ErrUserAborted), want: tracking.ResultCancelled},
		{name: "selection dismissed", runErr: tui.ErrFilterSelectCancelled, want: tracking.ResultCancelled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := useTrackingTestRoot(t, "shopware-cli")

			ctx := context.Background()
			if tt.interrupted {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			trackCommandExecution(ctx, []string{"project", "console"}, time.Now(), tt.runErr, "")

			require.Len(t, *events, 1)
			assert.Equal(t, tt.want, (*events)[0][tracking.TagResult])
		})
	}
}

func TestTrackCommandExecution_Tags(t *testing.T) {
	events := useTrackingTestRoot(t, "shopware-cli")
	ctx := system.WithInteraction(context.Background(), true)

	trackCommandExecution(ctx, []string{"project", "console"}, time.Now().Add(-2*time.Second), nil, "")

	require.Len(t, *events, 1)
	tags := (*events)[0]
	assert.Equal(t, runtime.GOOS, tags[tracking.TagOS])
	assert.Equal(t, "true", tags[tracking.TagIsTUI])
	assert.Equal(t, strconv.FormatBool(system.IsWSL()), tags[tracking.TagIsWSL])

	durationMS, err := strconv.ParseInt(tags[tracking.TagDurationMS], 10, 64)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, durationMS, int64(2000))
}

// clearCIProviderEnv unsets every variable CIProvider and IsCIEnvironment read.
func clearCIProviderEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"CI", "GITHUB_ACTIONS", "GITLAB_CI", "BITBUCKET_BUILD_NUMBER", "BITBUCKET_REPO_FULL_NAME", "JENKINS_URL", "BUILDKITE", "CIRCLECI", "DRONE", "TEAMCITY_VERSION", "TF_BUILD"} {
		t.Setenv(name, "")
	}
}

func TestTrackCommandExecution_CIProvider(t *testing.T) {
	clearCIProviderEnv(t)
	t.Setenv("GITHUB_ACTIONS", "true")
	events := useTrackingTestRoot(t, "shopware-cli")

	trackCommandExecution(context.Background(), []string{"project", "console"}, time.Now(), nil, "")

	require.Len(t, *events, 1)
	assert.Equal(t, "github", (*events)[0][tracking.TagCIProvider])
}

func TestTrackCommandExecution_NoCIProviderOutsideCI(t *testing.T) {
	clearCIProviderEnv(t)
	events := useTrackingTestRoot(t, "shopware-cli")

	trackCommandExecution(context.Background(), []string{"project", "console"}, time.Now(), nil, "")

	require.Len(t, *events, 1)
	assert.NotContains(t, (*events)[0], tracking.TagCIProvider)
}

func TestTrackCommandExecution_IsTUIFollowsInteraction(t *testing.T) {
	events := useTrackingTestRoot(t, "shopware-cli")
	ctx := system.WithInteraction(context.Background(), false)

	trackCommandExecution(ctx, []string{"project", "console"}, time.Now(), nil, "")

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

			trackCommandExecution(context.Background(), tt.args, time.Now(), nil, "")

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

	trackCommandExecution(ctx, []string{"project", "console"}, time.Now(), ctx.Err(), "")

	require.Len(t, *events, 1)
	assert.Equal(t, tracking.ResultCancelled, (*events)[0][tracking.TagResult])
	assert.NoError(t, trackCtxErr, "tracking must get a live context even after the command context was cancelled")
}

func writeShopwareLock(t *testing.T, version string) string {
	t.Helper()
	root := t.TempDir()
	lock := `{"packages":[{"name":"shopware/core","version":"` + version + `"}],"packages-dev":[]}`
	require.NoError(t, os.WriteFile(filepath.Join(root, "composer.lock"), []byte(lock), 0o644))
	return root
}

func TestProjectShopwareVersion(t *testing.T) {
	t.Setenv("PROJECT_ROOT", writeShopwareLock(t, "v6.7.11.0"))
	useTrackingTestRoot(t, "shopware-cli")

	assert.Equal(t, "6.7.11.0", projectShopwareVersion([]string{"project", "console"}))
	assert.Empty(t, projectShopwareVersion([]string{"project", "create"}), "project create runs where there is no project yet")
	assert.Empty(t, projectShopwareVersion([]string{"extension", "build-me"}), "not a project command")
	assert.Empty(t, projectShopwareVersion([]string{"project"}), "not a tracked invocation")

	t.Setenv("PROJECT_ROOT", t.TempDir())
	assert.Empty(t, projectShopwareVersion([]string{"project", "console"}), "no lock file")

	for lockVersion, want := range map[string]string{
		"v6.7.0.0-rc1":    "6.7.0.0-rc1",
		"dev-trunk":       "dev-trunk",
		"v6.7.0.0-RC1":    "",
		"6.6.x-dev":       "",
		"6.7.12.x-dev":    "",
		"dev-tmp-0c8ec10": "",
		"dev-feature-xyz": "",
		"6.7.11.0-dev":    "",
	} {
		t.Setenv("PROJECT_ROOT", writeShopwareLock(t, lockVersion))
		assert.Equal(t, want, projectShopwareVersion([]string{"project", "console"}), "lock version %s", lockVersion)
	}
}

func TestTrackCommandExecution_ShopwareVersionTag(t *testing.T) {
	events := useTrackingTestRoot(t, "shopware-cli")

	trackCommandExecution(context.Background(), []string{"project", "console"}, time.Now(), nil, "6.7.11.0")
	trackCommandExecution(context.Background(), []string{"project", "console"}, time.Now(), nil, "")

	require.Len(t, *events, 2)
	assert.Equal(t, "6.7.11.0", (*events)[0][tracking.TagShopwareVersion])
	assert.NotContains(t, (*events)[1], tracking.TagShopwareVersion)
}
