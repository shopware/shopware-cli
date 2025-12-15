package system

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExpandEnv(t *testing.T) {
	t.Setenv("TEST", "yea")

	cases := []struct {
		input string
		want  string
	}{
		{"${TEST}", "yea"},
		//nolint:dupword
		{"${TEST} ${TEST}", "yea yea"},
		{"$TEST", "$TEST"},
		{"$$TEST", "$$TEST"},
		{"$$TE$ST", "$$TE$ST"},
		{"${FOO_TEST}", ""},
		{"${FOO", "${FOO"},
	}

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			assert.Equal(t, c.want, ExpandEnv(c.input))
		})
	}
}
