package system

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePHPMemoryLimit(t *testing.T) {
	for input, want := range map[string]int64{
		"128M":       128 << 20,
		"512M":       512 << 20,
		"1G":         1 << 30,
		"512m":       512 << 20,
		"524288K":    512 << 20,
		"536870912":  512 << 20,
		"0x20000000": 512 << 20,
		"-1":         -1,
		"0":          0,
	} {
		t.Run(input, func(t *testing.T) {
			got, err := parsePHPMemoryLimit(input)
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}

	for _, input := range []string{"", "garbage", "-2", "9223372036854775807G"} {
		t.Run("invalid_"+input, func(t *testing.T) {
			_, err := parsePHPMemoryLimit(input)
			assert.Error(t, err)
		})
	}
}

func TestParsePHPMemoryLimitTrimsWhitespace(t *testing.T) {
	limit, err := parsePHPMemoryLimit(" 512M ")
	require.NoError(t, err)
	assert.Equal(t, int64(512<<20), limit)
}

func TestValidatePHPCreateMemoryOutput(t *testing.T) {
	for _, limit := range []string{"512M", "1G", "-1"} {
		output := fmt.Appendf(nil, `{"limit":%q,"ini":"/test/php.ini"}`, limit)
		assert.NoError(t, validatePHPCreateMemoryOutput(output, "/test/php"))
	}
	for _, output := range []string{
		`{"limit":"128M","ini":"/test/php.ini"}`,
		`{"limit":"bad"}`,
		`{}`,
		`invalid-json`,
	} {
		assert.Error(t, validatePHPCreateMemoryOutput([]byte(output), "/test/php"))
	}
}

func TestPHPCreateMemoryCommand(t *testing.T) {
	for _, override := range []string{"", "0", "128M", "512M", "-1"} {
		t.Run("override_"+override, func(t *testing.T) {
			cmd := phpCreateMemoryCommand(t.Context(), "/selected/php", override)
			assert.Equal(t, "/selected/php", cmd.Path)
			if override == "" || override == "0" {
				assert.Equal(t, "-r", cmd.Args[1])
			} else {
				assert.Equal(t, []string{"-d", "memory_limit=" + override, "-r"}, cmd.Args[1:4])
			}
		})
	}
}
