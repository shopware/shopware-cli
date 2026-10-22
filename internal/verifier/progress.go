package verifier

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Progress reports command activity independently of the final findings report.
// For concurrent use, its output writer must be safe for concurrent writes,
// such as os.Stderr, os.Stdout, or io.Discard.
type Progress struct {
	output io.Writer
}

// NewProgress reports activity on stderr for summary reports and fix commands,
// which use an empty format. Other report formats receive no progress output.
func NewProgress(format string) *Progress {
	var output io.Writer = os.Stderr
	if format != "" && !strings.EqualFold(format, "summary") {
		output = io.Discard
	}
	return &Progress{output: output}
}

// Start reports an operation immediately and returns its completion callback.
// Completion describes execution, not whether the operation found problems.
func (p *Progress) Start(title string) func(error) {
	started := time.Now()
	p.print("Running", title+"...")
	return func(err error) {
		state := "Finished"
		if err != nil {
			state = "Failed"
		}
		p.print(state, fmt.Sprintf("%s (%.1fs)", title, time.Since(started).Seconds()))
	}
}

func (p *Progress) print(state, title string) {
	_, _ = fmt.Fprintln(p.output, state, title)
}
