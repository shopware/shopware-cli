package ci

import (
	"io"
	"os"

	"github.com/mattn/go-isatty"
)

var defaultHelper = New(os.Stderr)

// Helper renders log sections for the current terminal or CI environment.
type Helper interface {
	Section(name string) Section
}

type Section interface {
	End()
}

// Start begins a section using the process output.
func Start(name string) Section {
	return defaultHelper.Section(name)
}

// New returns a section helper writing to output.
func New(output io.Writer) Helper {
	if output == nil {
		output = io.Discard
	}
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		return &githubActions{output: output}
	}
	if os.Getenv("GITLAB_CI") == "true" {
		return &gitlabCI{output: output}
	}
	return &terminalHelper{output: output, color: outputSupportsColor(output)}
}

func outputSupportsColor(output io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	for {
		if file, ok := output.(*os.File); ok {
			return isatty.IsTerminal(file.Fd()) || isatty.IsCygwinTerminal(file.Fd())
		}
		wrapper, ok := output.(interface{ UnwrapWriter() io.Writer })
		if !ok {
			return false
		}
		output = wrapper.UnwrapWriter()
	}
}
