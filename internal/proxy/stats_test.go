package proxy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseDockerMemUsage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"512MiB / 7.6GiB", 512 << 20, true},
		{"2GiB / 7.6GiB", 2 << 30, true},
		{"256KiB / 7.6GiB", 256 << 10, true},
		{"1.5GiB / 7.6GiB", int64(1.5 * (1 << 30)), true},
		{"128B / 7.6GiB", 128, true},
		{"  45MiB  ", 45 << 20, true},
		{"garbage", 0, false},
		{"", 0, false},
	}

	for _, c := range cases {
		got, ok := parseDockerMemUsage(c.in)
		assert.Equal(t, c.ok, ok, c.in)
		if c.ok {
			assert.Equal(t, c.want, got, c.in)
		}
	}
}
