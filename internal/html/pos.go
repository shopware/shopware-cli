package html

import "fmt"

// Pos is a byte-based position in the source.
// Line and Column are 1-based; columns count bytes (not runes).
type Pos struct {
	Offset int
	Line   int
	Column int
}

func (p Pos) String() string {
	return fmt.Sprintf("%d:%d", p.Line, p.Column)
}

// posTracker advances a Pos one byte at a time through source.
type posTracker struct {
	src string
	cur Pos
}

func newPosTracker(src string) *posTracker {
	return &posTracker{src: src, cur: Pos{Offset: 0, Line: 1, Column: 1}}
}

// advance moves the position by n bytes, updating line/column.
func (t *posTracker) advance(n int) {
	for i := 0; i < n && t.cur.Offset < len(t.src); i++ {
		b := t.src[t.cur.Offset]
		t.cur.Offset++
		if b == '\n' {
			t.cur.Line++
			t.cur.Column = 1
		} else {
			t.cur.Column++
		}
	}
}

// pos returns the current Pos.
func (t *posTracker) pos() Pos { return t.cur }
