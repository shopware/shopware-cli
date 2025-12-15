package verifier

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPhpStan_isUselessDeprecation(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    bool
	}{
		{
			name:    "message without tag version",
			message: "Some deprecated method without version tag",
			want:    true,
		},
		{
			name:    "message with tag version",
			message: "Method deprecated since tag:v6.5.0",
			want:    false,
		},
		{
			name:    "parameter removal message with tag",
			message: "Parameter $foo will be removed in tag:v6.6.0",
			want:    true,
		},
		{
			name:    "parameter removal message without tag",
			message: "Parameter $bar will be removed",
			want:    true,
		},
		{
			name:    "return type change reason with tag",
			message: "Deprecated method tag:v6.5.0 reason:return-type-change",
			want:    true,
		},
		{
			name:    "new optional parameter reason with tag",
			message: "Deprecated constructor tag:v6.5.0 reason:new-optional-parameter",
			want:    true,
		},
		{
			name:    "valid deprecation with tag",
			message: "Method Foo::bar() is deprecated since tag:v6.5.0 and will be removed",
			want:    false,
		},
		{
			name:    "multiple version tags",
			message: "Deprecated since tag:v6.4.0, updated in tag:v6.5.0",
			want:    false,
		},
		{
			name:    "invalid version tag format",
			message: "Method deprecated since tag:invalid-version",
			want:    true,
		},
	}

	p := PhpStan{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.isUselessDeprecation(tt.message)
			assert.Equal(t, tt.want, got)
		})
	}
}
