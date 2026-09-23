package ci

import (
	"fmt"
	"io"
	"time"
)

type githubActions struct {
	output io.Writer
}

type githubActionsSection struct {
	name   string
	start  time.Time
	output io.Writer
}

func (g *githubActions) Section(name string) Section {
	_, _ = fmt.Fprintf(g.output, "::group::%s\n", name)
	return githubActionsSection{
		name:   name,
		start:  time.Now(),
		output: g.output,
	}
}

func (s githubActionsSection) End() {
	_, _ = fmt.Fprintf(s.output, "%s finished in %s\n", s.name, time.Since(s.start).Round(time.Millisecond))
	_, _ = fmt.Fprintln(s.output, "::endgroup::")
}
