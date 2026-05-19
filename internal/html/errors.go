package html

import (
	"fmt"
	"strings"
)

// ParseError is a parser-level error with source location.
type ParseError struct {
	Filename string
	Pos      Pos
	Msg      string
	Related  []RelatedLoc
	Source   string
}

// RelatedLoc points to another location relevant to a ParseError
// (e.g. the opening tag for an unmatched closer).
type RelatedLoc struct {
	Pos Pos
	Msg string
}

func (e *ParseError) Error() string {
	prefix := e.Filename
	if prefix == "" {
		prefix = "<input>"
	}
	return fmt.Sprintf("%s:%d:%d: %s", prefix, e.Pos.Line, e.Pos.Column, e.Msg)
}

// PrettyError returns the error with a source snippet, suitable for terminal display.
func (e *ParseError) PrettyError() string {
	var b strings.Builder
	b.WriteString(e.Error())
	b.WriteString("\n")
	b.WriteString(snippet(e.Source, e.Pos))
	for _, r := range e.Related {
		fmt.Fprintf(&b, "  = note: %s at %d:%d\n", r.Msg, r.Pos.Line, r.Pos.Column)
	}
	return b.String()
}

// snippet renders 1-2 lines of context around pos with a caret marker.
func snippet(src string, pos Pos) string {
	if src == "" || pos.Line < 1 {
		return ""
	}
	lines := strings.Split(src, "\n")
	if pos.Line > len(lines) {
		return ""
	}
	line := lines[pos.Line-1]
	col := pos.Column
	if col < 1 {
		col = 1
	}
	caret := strings.Repeat(" ", col-1) + "^"
	return fmt.Sprintf("  %4d | %s\n       | %s\n", pos.Line, line, caret)
}

// errAt builds a ParseError at the given position.
func errAt(src, filename string, pos Pos, format string, args ...any) *ParseError {
	return &ParseError{
		Filename: filename,
		Pos:      pos,
		Msg:      fmt.Sprintf(format, args...),
		Source:   src,
	}
}
