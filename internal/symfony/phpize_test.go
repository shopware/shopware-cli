package symfony

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPhpize(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  any
	}{
		{name: "empty string", value: "", want: ""},
		{name: "null", value: "null", want: nil},
		{name: "null mixed case", value: "NULL", want: nil},
		{name: "true", value: "true", want: true},
		{name: "true mixed case", value: "True", want: true},
		{name: "false", value: "false", want: false},
		{name: "integer", value: "42", want: int64(42)},
		{name: "negative integer", value: "-42", want: int64(-42)},
		{name: "zero", value: "0", want: int64(0)},
		{name: "octal", value: "0700", want: int64(448)},
		{name: "negative octal", value: "-0700", want: int64(-448)},
		{name: "invalid octal stays string", value: "08", want: "08"},
		{name: "double zero prefix stays string", value: "00700", want: "00700"},
		{name: "binary", value: "0b101", want: int64(5)},
		{name: "hex", value: "0x1A", want: int64(26)},
		{name: "float", value: "1.5", want: 1.5},
		{name: "negative float", value: "-1.5", want: -1.5},
		{name: "exponent", value: "1e3", want: 1000.0},
		{name: "plus prefixed number", value: "+12", want: 12.0},
		{name: "plain string", value: "hello", want: "hello"},
		{name: "parameter placeholder", value: "%kernel.debug%", want: "%kernel.debug%"},
		{name: "yes is not a bool", value: "yes", want: "yes"},
		{name: "huge number stays string", value: "99999999999999999999", want: "99999999999999999999"},
		{name: "version-like string", value: "1.2.3", want: "1.2.3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, phpize(tt.value))
		})
	}
}
