package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPtr(t *testing.T) {
	p := Ptr(false)
	require.NotNil(t, p)
	assert.False(t, *p)
}

func TestAsMsg(t *testing.T) {
	type sample struct{ ID string }

	got, ok := As[sample](sample{ID: "a"})
	require.True(t, ok)
	assert.Equal(t, "a", got.ID)

	_, ok = As[struct{ X int }](sample{ID: "a"})
	assert.False(t, ok)
}

func TestContentAs(t *testing.T) {
	var content Content = StaticContent{Text: "x"}
	got, ok := ContentAs[StaticContent](content)
	require.True(t, ok)
	assert.Equal(t, "x", got.Text)
}

func TestClampFilterMapIndex(t *testing.T) {
	assert.Equal(t, 3, Clamp(5, 0, 3))
	assert.Equal(t, 0, Clamp(-1, 0, 3))
	assert.Equal(t, 2, Clamp(2, 3, 0), "swapped bounds")

	assert.Equal(t, []int{2, 4}, Filter([]int{1, 2, 3, 4}, func(n int) bool { return n%2 == 0 }))
	assert.Equal(t, []int{1, 3}, FilterIndex([]int{1, 2, 3, 4}, func(n int) bool { return n%2 == 0 }))
	assert.Equal(t, []string{"1", "2"}, Map([]int{1, 2}, func(n int) string { return string(rune('0' + n)) }))

	assert.Equal(t, 1, Index([]string{"a", "b"}, "b"))
	assert.Equal(t, -1, Index([]string{"a", "b"}, "c"))

	v, ok := At([]int{10, 20}, 1)
	require.True(t, ok)
	assert.Equal(t, 20, v)
	_, ok = At([]int{10, 20}, 5)
	assert.False(t, ok)
}
