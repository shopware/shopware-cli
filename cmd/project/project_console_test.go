package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripConsoleFlags(t *testing.T) {
	cases := []struct {
		name        string
		args        []string
		expected    []string
		expectedEnv string
	}{
		{
			name:     "no flags",
			args:     []string{"cache:clear"},
			expected: []string{"cache:clear"},
		},
		{
			name:        "env before command",
			args:        []string{"--env", "production", "cache:clear"},
			expected:    []string{"cache:clear"},
			expectedEnv: "production",
		},
		{
			name:        "env equals form",
			args:        []string{"--env=production", "cache:clear"},
			expected:    []string{"cache:clear"},
			expectedEnv: "production",
		},
		{
			name:        "shorthand",
			args:        []string{"-e", "production", "cache:clear"},
			expected:    []string{"cache:clear"},
			expectedEnv: "production",
		},
		{
			name:     "symfony env after command stays untouched",
			args:     []string{"cache:clear", "--env=prod"},
			expected: []string{"cache:clear", "--env=prod"},
		},
		{
			name:        "cli env before, symfony env after",
			args:        []string{"--env", "production", "cache:clear", "--env=prod", "-e", "dev"},
			expected:    []string{"cache:clear", "--env=prod", "-e", "dev"},
			expectedEnv: "production",
		},
		{
			name:     "double dash passes everything through",
			args:     []string{"--", "--env=prod", "cache:clear"},
			expected: []string{"--env=prod", "cache:clear"},
		},
		{
			name:     "unknown flags before command are forwarded",
			args:     []string{"-v", "cache:clear"},
			expected: []string{"-v", "cache:clear"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			environmentName = ""
			t.Cleanup(func() { environmentName = "" })

			rest := stripConsoleFlags(tc.args)

			assert.Equal(t, tc.expected, rest)
			assert.Equal(t, tc.expectedEnv, environmentName)
		})
	}
}
