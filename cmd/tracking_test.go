package cmd

import (
	"context"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/tracking"
	"github.com/shopware/shopware-cli/internal/tui"
)

func TestExecuteTracksConsoleAcrossBinaryNames(t *testing.T) {
	tests := []struct {
		name        string
		argv        []string
		consoleArgs []string
	}{
		{
			name:        "regular binary",
			argv:        []string{"shopware-cli", "project", "console", "cache:clear"},
			consoleArgs: []string{"cache:clear"},
		},
		{
			name:        "swx alias",
			argv:        []string{"/usr/local/bin/swx", "cache:clear"},
			consoleArgs: []string{"cache:clear"},
		},
		{
			name:        "swx default command",
			argv:        []string{"swx"},
			consoleArgs: []string{"list"},
		},
		{
			name:        "windows alias",
			argv:        []string{`C:\tools\swx.exe`, "cache:clear"},
			consoleArgs: []string{"cache:clear"},
		},
		{
			name:        "renamed binary",
			argv:        []string{"custom-cli", "project", "console", "cache:clear"},
			consoleArgs: []string{"cache:clear"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalRoot, originalArgs, originalTrack := rootCmd, os.Args, trackEvent
			originalUpdateAvailable := tui.UpdateAvailable
			t.Cleanup(func() {
				rootCmd, os.Args, trackEvent = originalRoot, originalArgs, originalTrack
				tui.UpdateAvailable = originalUpdateAvailable
			})

			rootCmd = &cobra.Command{Use: "shopware-cli"}
			projectCmd := &cobra.Command{Use: "project"}
			var executedArgs []string
			projectCmd.AddCommand(&cobra.Command{
				Use:                "console",
				DisableFlagParsing: true,
				RunE: func(_ *cobra.Command, args []string) error {
					executedArgs = args
					return nil
				},
			})
			rootCmd.AddCommand(projectCmd)
			os.Args = tt.argv

			var events []map[string]string
			trackEvent = func(_ context.Context, event string, tags map[string]string) {
				assert.Equal(t, tracking.EventCommand, event)
				events = append(events, tags)
			}

			assert.Zero(t, Execute(t.Context()))
			assert.Equal(t, tt.consoleArgs, executedArgs)
			if assert.Len(t, events, 1) {
				assert.Equal(t, "project.console", events[0][tracking.TagCommandName])
				assert.Equal(t, tracking.ResultSuccess, events[0][tracking.TagResult])
			}
		})
	}
}
