package ci

import (
	"fmt"
	"io"
	"time"
)

type terminalHelper struct {
	output io.Writer
	color  bool
}

type terminalSection struct {
	name   string
	start  time.Time
	output io.Writer
	color  bool
}

func (d *terminalHelper) Section(name string) Section {
	if d.color {
		_, _ = fmt.Fprintf(d.output, "\n\x1b[1;36m━━ %s\x1b[0m\n", name)
	} else {
		_, _ = fmt.Fprintf(d.output, "\n--- %s ---\n", name)
	}
	return terminalSection{
		name:   name,
		start:  time.Now(),
		output: d.output,
		color:  d.color,
	}
}

func (d terminalSection) End() {
	elapsed := time.Since(d.start).Round(time.Millisecond)
	if d.color {
		_, _ = fmt.Fprintf(d.output, "\x1b[32m✓ %s finished in %s\x1b[0m\n", d.name, elapsed)
	} else {
		_, _ = fmt.Fprintf(d.output, "--- %s finished in %s ---\n", d.name, elapsed)
	}
}
