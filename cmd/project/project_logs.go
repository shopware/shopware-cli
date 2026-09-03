package project

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/tui"
)

var projectLogsCmd = &cobra.Command{
	Use:   "logs [filename]",
	Short: "Show Shopware application logs from var/log/",
	Long:  "Show the last lines of a Shopware log file. Without arguments, shows the most recently modified log file. Use --list to discover available log files.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectRoot, err := findClosestShopwareProject(false)
		if err != nil {
			return err
		}

		cmdExecutor, err := resolveExecutor(cmd, projectRoot)
		if err != nil {
			return err
		}

		return runProjectLogs(cmd, args, cmdExecutor)
	},
}

func runProjectLogs(cmd *cobra.Command, args []string, cmdExecutor executor.Executor) error {
	lines, _ := cmd.Flags().GetInt("lines")
	if lines < 0 {
		return fmt.Errorf("invalid value %d for --lines: must not be negative", lines)
	}

	files, err := cmdExecutor.AvailableLogFiles(cmd.Context())
	if err != nil {
		return err
	}

	list, _ := cmd.Flags().GetBool("list")
	if list {
		return printLogFileList(files)
	}

	if len(files) == 0 {
		return errors.New("no log files found in var/log")
	}

	target := files[0].Name
	if len(args) > 0 {
		target = ""
		for _, f := range files {
			if f.Name == args[0] {
				target = f.Name
				break
			}
		}

		if target == "" {
			return fmt.Errorf("log file not found: %s", args[0])
		}
	}

	follow, _ := cmd.Flags().GetBool("follow")

	return cmdExecutor.GetLog(cmd.Context(), target, lines, follow, cmd.OutOrStdout())
}

func printLogFileList(files []executor.LogFile) error {
	if len(files) == 0 {
		fmt.Println(tui.DimText.Render("No log files found."))
		return nil
	}

	rows := make([][]string, 0, len(files))
	for _, f := range files {
		rows = append(rows, []string{f.Name, formatSize(f.Size), f.ModTime.Format("2006-01-02 15:04:05")})
	}
	tui.PrintTable([]string{"File", "Size", "Modified"}, rows)

	return nil
}

func formatSize(bytes int64) string {
	switch {
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func init() {
	projectRootCmd.AddCommand(projectLogsCmd)
	projectLogsCmd.Flags().Int("lines", 100, "Number of lines to show")
	projectLogsCmd.Flags().BoolP("follow", "f", false, "Follow the log file for new output")
	projectLogsCmd.Flags().BoolP("list", "l", false, "List available log files")
}
