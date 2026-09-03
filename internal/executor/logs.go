package executor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// LogFile describes a log file in the project's var/log directory.
type LogFile struct {
	Name    string
	Size    int64
	ModTime time.Time
}

// listLogFilesPHP returns a PHP snippet printing the *.log files of logDir
// as JSON. PHP is used instead of ls/stat so the output format is identical
// on every platform an executor can target (Docker, SSH remotes).
func listLogFilesPHP(logDir string) string {
	return `$files = []; foreach (glob(` + strconv.Quote(logDir+"/*.log") + `) ?: [] as $f) { $files[] = ["name" => basename($f), "size" => filesize($f), "mtime" => filemtime($f)]; } echo json_encode($files);`
}

// parseLogFiles decodes the JSON printed by listLogFilesPHP and sorts the
// files most recently modified first.
func parseLogFiles(out []byte) ([]LogFile, error) {
	var entries []struct {
		Name  string `json:"name"`
		Size  int64  `json:"size"`
		Mtime int64  `json:"mtime"`
	}
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("could not parse log file list: %w", err)
	}

	files := make([]LogFile, 0, len(entries))
	for _, e := range entries {
		files = append(files, LogFile{Name: e.Name, Size: e.Size, ModTime: time.Unix(e.Mtime, 0)})
	}

	sortLogFiles(files)

	return files, nil
}

// localLogFiles lists the .log files in dir, most recently modified first.
func localLogFiles(dir string) ([]LogFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("could not read log directory: %w", err)
	}

	var files []LogFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		files = append(files, LogFile{Name: entry.Name(), Size: info.Size(), ModTime: info.ModTime()})
	}

	sortLogFiles(files)

	return files, nil
}

func sortLogFiles(files []LogFile) {
	slices.SortFunc(files, func(a, b LogFile) int {
		return b.ModTime.Compare(a.ModTime)
	})
}

// logFilePath joins the var/log directory of a project root with a log file
// name. The name always comes from AvailableLogFiles (a basename), so this
// cannot escape the log directory.
func logFilePath(projectRoot, file string) string {
	return filepath.Join(projectRoot, "var", "log", file)
}

// tailArgs builds the tail argument list for GetLog implementations.
func tailArgs(file string, lines int, follow bool) []string {
	args := []string{"-n", strconv.Itoa(lines)}
	if follow {
		args = append(args, "-f")
	}

	return append(args, file)
}
