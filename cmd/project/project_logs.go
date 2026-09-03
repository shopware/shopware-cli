package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/tui"
)

// listLogFilesPHP prints the var/log *.log files of the project as JSON. PHP
// is used instead of ls/stat so the output format is identical on every
// platform an executor can target (local, Docker, SSH remotes).
const listLogFilesPHP = `$files = []; foreach (glob("var/log/*.log") ?: [] as $f) { $files[] = ["name" => basename($f), "size" => filesize($f), "mtime" => filemtime($f)]; } echo json_encode($files);`

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

		files, err := findLogFiles(cmd, cmdExecutor)
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

		target := files[0].name
		if len(args) > 0 {
			target = ""
			for _, f := range files {
				if f.name == args[0] {
					target = f.name
					break
				}
			}

			if target == "" {
				return fmt.Errorf("log file not found: %s", args[0])
			}
		}

		lines, _ := cmd.Flags().GetInt("lines")
		follow, _ := cmd.Flags().GetBool("follow")

		tailArgs := []string{"-n", strconv.Itoa(lines)}
		if follow {
			tailArgs = append(tailArgs, "-f")
		}
		tailArgs = append(tailArgs, "var/log/"+target)

		p := cmdExecutor.Command(cmd.Context(), "tail", tailArgs...)
		p.Cmd.Stdout = cmd.OutOrStdout()
		p.Cmd.Stderr = cmd.ErrOrStderr()

		return p.Run()
	},
}

type logFileInfo struct {
	name    string
	size    int64
	modTime time.Time
}

func findLogFiles(cmd *cobra.Command, cmdExecutor executor.Executor) ([]logFileInfo, error) {
	out, err := cmdExecutor.PHPCommand(cmd.Context(), "-r", listLogFilesPHP).Output()
	if err != nil {
		return nil, fmt.Errorf("could not list log files: %w", err)
	}

	return parseLogFiles(out)
}

func parseLogFiles(out []byte) ([]logFileInfo, error) {
	var entries []struct {
		Name  string `json:"name"`
		Size  int64  `json:"size"`
		Mtime int64  `json:"mtime"`
	}
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("could not parse log file list: %w", err)
	}

	files := make([]logFileInfo, 0, len(entries))
	for _, e := range entries {
		files = append(files, logFileInfo{name: e.Name, size: e.Size, modTime: time.Unix(e.Mtime, 0)})
	}

	// Sort by modification time, most recent first
	slices.SortFunc(files, func(a, b logFileInfo) int {
		return b.modTime.Compare(a.modTime)
	})

	return files, nil
}

func printLogFileList(files []logFileInfo) error {
	if len(files) == 0 {
		fmt.Println(tui.DimText.Render("No log files found."))
		return nil
	}

	rows := make([][]string, 0, len(files))
	for _, f := range files {
		rows = append(rows, []string{f.name, formatSize(f.size), f.modTime.Format("2006-01-02 15:04:05")})
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
