package project

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/tui"
)

var projectLogsCmd = &cobra.Command{
	Use:   "logs [filename]",
	Short: "Show Shopware application logs from var/log/",
	Long:  "Show the last lines of a Shopware log file. Without arguments, shows the most recently modified log file. Use --list to discover available log files.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectRoot, err := findClosestShopwareProject()
		if err != nil {
			return err
		}

		logDir := filepath.Join(projectRoot, "var", "log")

		list, _ := cmd.Flags().GetBool("list")
		if list {
			return listLogFiles(logDir)
		}

		files, err := findLogFiles(logDir)
		if err != nil {
			return err
		}

		if len(files) == 0 {
			return fmt.Errorf("no log files found in %s", logDir)
		}

		var target string
		if len(args) > 0 {
			target = filepath.Join(logDir, args[0])
			if _, err := os.Stat(target); err != nil {
				return fmt.Errorf("log file not found: %s", args[0])
			}
		} else {
			// Most recently modified file
			target = files[0].path
		}

		lines, _ := cmd.Flags().GetInt("lines")
		follow, _ := cmd.Flags().GetBool("follow")

		if follow {
			return tailFollow(cmd, target, lines)
		}

		return printLastLines(target, lines)
	},
}

type logFileInfo struct {
	path    string
	name    string
	size    int64
	modTime time.Time
}

func findLogFiles(logDir string) ([]logFileInfo, error) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return nil, fmt.Errorf("could not read log directory: %w", err)
	}

	var files []logFileInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		files = append(files, logFileInfo{
			path:    filepath.Join(logDir, entry.Name()),
			name:    entry.Name(),
			size:    info.Size(),
			modTime: info.ModTime(),
		})
	}

	// Sort by modification time, most recent first
	slices.SortFunc(files, func(a, b logFileInfo) int {
		return b.modTime.Compare(a.modTime)
	})

	return files, nil
}

func listLogFiles(logDir string) error {
	files, err := findLogFiles(logDir)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		fmt.Println(tui.DimText.Render("No log files found."))
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	_, _ = fmt.Fprintln(w, tui.BoldText.Render("File")+"\t"+tui.BoldText.Render("Size")+"\t"+tui.BoldText.Render("Modified"))

	for _, f := range files {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", f.name, formatSize(f.size), f.modTime.Format("2006-01-02 15:04:05"))
	}

	return w.Flush()
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

func printLastLines(path string, n int) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	// Use a ring buffer to keep the last N lines
	ring := make([]string, 0, n)
	for scanner.Scan() {
		if len(ring) < n {
			ring = append(ring, scanner.Text())
		} else {
			copy(ring, ring[1:])
			ring[n-1] = scanner.Text()
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	for _, line := range ring {
		fmt.Println(line)
	}

	return nil
}

func tailFollow(cmd *cobra.Command, path string, n int) error {
	tailCmd := exec.CommandContext(cmd.Context(), "tail", "-n", fmt.Sprintf("%d", n), "-f", path)
	tailCmd.Stdout = cmd.OutOrStdout()
	tailCmd.Stderr = cmd.ErrOrStderr()

	return tailCmd.Run()
}

func init() {
	projectRootCmd.AddCommand(projectLogsCmd)
	projectLogsCmd.Flags().Int("lines", 100, "Number of lines to show")
	projectLogsCmd.Flags().BoolP("follow", "f", false, "Follow the log file for new output")
	projectLogsCmd.Flags().BoolP("list", "l", false, "List available log files")
}
