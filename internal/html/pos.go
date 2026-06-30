package html

import (
	"fmt"
	"strings"
)

// Pos is a byte-based position in the source. Line is 1-based.
//
// Column is intentionally NOT stored: it is needed only on the cold error path,
// so carrying it in every token (and recomputing it on every advance) is pure
// overhead. Compute it on demand with ColumnIn, which derives it from
// Offset and the source. Dropping the field shrinks Pos from 24 to 16 bytes,
// which in turn shrinks the token struct that embeds it — and the token stream
// is copied on every peek/advance/emit, so this directly cuts CPU.
type Pos struct {
	Offset int
	Line   int
}

// String renders the position as "line:offset". Column is omitted because it
// can only be derived with the source string (see ColumnIn); use that when a
// column is needed.
func (p Pos) String() string {
	return fmt.Sprintf("line %d (offset %d)", p.Line, p.Offset)
}

// ColumnIn returns the 1-based byte column of this position within src.
func (p Pos) ColumnIn(src string) int {
	if p.Offset <= 0 || p.Offset > len(src) {
		return 1
	}
	if nl := strings.LastIndexByte(src[:p.Offset], '\n'); nl != -1 {
		return p.Offset - nl
	}
	return p.Offset + 1
}

// posTracker tracks the current Pos as the lexer advances through source.
type posTracker struct {
	src string
	cur Pos
}

// advance moves the position forward by n bytes. It counts any newlines in the
// skipped span in one SIMD-accelerated pass (strings.Count) instead of looping
// byte by byte, and no longer tracks column — column is derived lazily from the
// offset only when an error needs it.
func (t *posTracker) advance(n int) {
	end := t.cur.Offset + n
	if end > len(t.src) {
		end = len(t.src)
	}
	if end <= t.cur.Offset {
		return
	}
	t.cur.Line += strings.Count(t.src[t.cur.Offset:end], "\n")
	t.cur.Offset = end
}

// pos returns the current Pos.
func (t *posTracker) pos() Pos { return t.cur }
