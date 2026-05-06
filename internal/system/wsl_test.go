package system

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsWSLWindowsMount(t *testing.T) {
	t.Parallel()

	if !IsWSL() {
		assert.False(t, IsWSLWindowsMount("/mnt/c/project"), "non-WSL systems should return false")
		assert.False(t, IsWSLWindowsMount("/home/user/project"), "non-WSL systems should return false")
		return
	}

	tests := []struct {
		path     string
		expected bool
	}{
		{"/mnt/c/project", true},
		{"/mnt/c/Users/test/project", true},
		{"/mnt/d/workspace", true},
		{"/mnt/z/foo", true},
		{"/home/user/project", false},
		{"/root/project", false},
		{"/mnt/", false},
		{"/mnt/ab/test", false},
		{"/mnt/1/test", false},
	}

	for _, tc := range tests {
		assert.Equal(t, tc.expected, IsWSLWindowsMount(tc.path), "path: %s", tc.path)
	}
}
