package ci

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

var gitlabSectionRegex = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

type gitlabCI struct {
	output io.Writer
}

type gitlabCISection struct {
	name   string
	start  time.Time
	output io.Writer
}

func gitlabSectionId(name string) string {
	return gitlabSectionRegex.ReplaceAllString(strings.ToLower(name), "_")
}

func (g *gitlabCI) Section(name string) Section {
	sectionId := gitlabSectionId(name)
	_, _ = fmt.Fprintf(g.output, "section_start:%d:%s\r\x1b[0K%s\n", time.Now().Unix(), sectionId, name)
	return gitlabCISection{
		name:   name,
		start:  time.Now(),
		output: g.output,
	}
}

func (g gitlabCISection) End() {
	sectionId := gitlabSectionId(g.name)
	_, _ = fmt.Fprintf(g.output, "%s finished in %s\n", g.name, time.Since(g.start).Round(time.Millisecond))
	_, _ = fmt.Fprintf(g.output, "section_end:%d:%s\r\x1b[0K\n", time.Now().Unix(), sectionId)
}
