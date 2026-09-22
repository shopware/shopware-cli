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
	fmt.Fprintf(g.output, "::group::%s\n", name) //nolint:errcheck // log formatting is best-effort
	return githubActionsSection{
		name:   name,
		start:  time.Now(),
		output: g.output,
	}
}

func (s githubActionsSection) End() {
	fmt.Fprintf(s.output, "%s finished in %s\n", s.name, time.Since(s.start).Round(time.Millisecond)) //nolint:errcheck // log formatting is best-effort
	fmt.Fprintln(s.output, "::endgroup::")                                                            //nolint:errcheck // log formatting is best-effort
}
