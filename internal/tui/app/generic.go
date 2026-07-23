package app

import (
	"cmp"

	tea "charm.land/bubbletea/v2"
)

// Ptr returns a pointer to v.
// Useful for optional Options fields:
//
//	app.New(app.Options{AltScreen: app.Ptr(false), FullscreenOverlay: app.Ptr(true)})
func Ptr[T any](v T) *T {
	return &v
}

// As type-asserts a Bubble Tea message.
//
//	if res, ok := app.As[versionPickedMsg](msg); ok { ... }
func As[T any](msg tea.Msg) (T, bool) {
	v, ok := msg.(T)
	return v, ok
}

// ContentAs type-asserts Content after Update.
func ContentAs[T Content](c Content) (T, bool) {
	v, ok := c.(T)
	return v, ok
}

// Clamp returns v constrained to [lo, hi]. If lo > hi the bounds are swapped.
func Clamp[T cmp.Ordered](v, lo, hi T) T {
	if lo > hi {
		lo, hi = hi, lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Filter returns a new slice of elements for which keep returns true.
func Filter[T any](in []T, keep func(T) bool) []T {
	if len(in) == 0 {
		return nil
	}
	out := make([]T, 0, len(in))
	for _, v := range in {
		if keep(v) {
			out = append(out, v)
		}
	}
	return out
}

// FilterIndex returns indices of elements for which keep returns true.
// Used by palette/picker style UIs that track filtered row indexes.
func FilterIndex[T any](in []T, keep func(T) bool) []int {
	if len(in) == 0 {
		return nil
	}
	out := make([]int, 0, len(in))
	for i, v := range in {
		if keep(v) {
			out = append(out, i)
		}
	}
	return out
}

// Map applies f to every element and returns the results.
func Map[T, U any](in []T, f func(T) U) []U {
	if len(in) == 0 {
		return nil
	}
	out := make([]U, len(in))
	for i, v := range in {
		out[i] = f(v)
	}
	return out
}

// IndexFunc returns the first index where f is true, or -1.
func IndexFunc[T any](in []T, f func(T) bool) int {
	for i, v := range in {
		if f(v) {
			return i
		}
	}
	return -1
}

// Index returns the first index of v, or -1.
func Index[T comparable](in []T, v T) int {
	return IndexFunc(in, func(x T) bool { return x == v })
}

// At returns the element at i, or the zero value if out of range.
func At[T any](in []T, i int) (T, bool) {
	var zero T
	if i < 0 || i >= len(in) {
		return zero, false
	}
	return in[i], true
}
