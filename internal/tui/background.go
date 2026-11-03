package tui

import (
	"image/color"
	"os"
	"strconv"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
)

// hasDarkBackground detects the terminal background once, on first use.
//
// The detection writes an OSC 11 (background colour) query followed by a DA1
// query to the terminal and reads until the DA1 reply arrives. Running it at
// package init (as lipgloss' compat package does) means every invocation,
// including --help, sends the query.
var hasDarkBackground = sync.OnceValue(func() bool {
	return detectDarkBackground(os.Getenv, func() bool {
		return lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
	})
})

// HasDarkBackground reports whether the terminal has a dark background. The
// terminal is queried at most once per process. Call it before starting a
// Bubble Tea program so the query never competes with the program for stdin.
func HasDarkBackground() bool {
	return hasDarkBackground()
}

// detectDarkBackground decides the background without querying the terminal
// where a query is unreliable, and falls back to query otherwise.
func detectDarkBackground(getenv func(string) string, query func() bool) bool {
	if dark, ok := darkFromColorFgBg(getenv("COLORFGBG")); ok {
		return dark
	}

	// GNU screen answers the DA1 query itself but forwards OSC 11 to the
	// outer terminal. The DA1 reply then often arrives first, the query
	// returns, and the late OSC 11 reply lands on the shell as
	// "^[]11;rgb:0000/0000/0000^G" (shopware/shopware-cli#1781).
	if getenv("STY") != "" {
		return true
	}

	return query()
}

// darkFromColorFgBg parses COLORFGBG ("fg;bg" or "fg;default;bg"), as set by
// rxvt, Konsole and others. Background colours 7 (white) and 15 (bright
// white) are light; every other ANSI index is dark.
func darkFromColorFgBg(value string) (dark bool, ok bool) {
	if value == "" {
		return false, false
	}

	parts := strings.Split(value, ";")
	bg, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil || bg < 0 || bg > 15 {
		return false, false
	}

	return bg != 7 && bg != 15, true
}

// AdaptiveColor picks Light or Dark depending on the terminal background. The
// background is resolved when the colour is first rendered, not when it is
// declared, so declaring package-level styles does not query the terminal.
type AdaptiveColor struct {
	Light color.Color
	Dark  color.Color
}

// RGBA implements color.Color.
func (c AdaptiveColor) RGBA() (r, g, b, a uint32) {
	if HasDarkBackground() {
		return c.Dark.RGBA()
	}
	return c.Light.RGBA()
}
