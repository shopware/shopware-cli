package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIgnoreMatches(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name        string
		result      CheckResult
		ignore      ToolConfigIgnore
		pathMatches func(resultPath, ignorePath string) bool
		want        bool
	}

	testCases := []testCase{
		{
			name:   "identifier only",
			result: CheckResult{Identifier: "metadata.description.length.de-DE"},
			ignore: ToolConfigIgnore{Identifier: "metadata.description"},
			pathMatches: func(_, _ string) bool {
				return false
			},
			want: true,
		},
		{
			name:   "identifier only metadata label",
			result: CheckResult{Identifier: "metadata.label.translation.de-DE"},
			ignore: ToolConfigIgnore{Identifier: "metadata.label"},
			pathMatches: func(_, _ string) bool {
				return false
			},
			want: true,
		},
		{
			name:   "identifier and path",
			result: CheckResult{Identifier: "test.rule", Path: "composer.json"},
			ignore: ToolConfigIgnore{Identifier: "test.rule", Path: "composer.json"},
			pathMatches: func(resultPath, ignorePath string) bool {
				return resultPath == ignorePath
			},
			want: true,
		},
		{
			name:   "identifier and message",
			result: CheckResult{Identifier: "test.rule", Message: "contains this"},
			ignore: ToolConfigIgnore{Identifier: "test.rule", Message: "this"},
			pathMatches: func(_, _ string) bool {
				return false
			},
			want: true,
		},
		{
			name:   "identifier and message with non matching path",
			result: CheckResult{Identifier: "test.rule", Path: "a.php", Message: "contains this"},
			ignore: ToolConfigIgnore{Identifier: "test.rule", Path: "b.php", Message: "this"},
			pathMatches: func(resultPath, ignorePath string) bool {
				return resultPath == ignorePath
			},
			want: false,
		},
		{
			name:   "message only",
			result: CheckResult{Path: "composer.json", Message: "missing key"},
			ignore: ToolConfigIgnore{Path: "composer.json", Message: "missing"},
			pathMatches: func(resultPath, ignorePath string) bool {
				return resultPath == ignorePath
			},
			want: true,
		},
		{
			name:   "no match",
			result: CheckResult{Identifier: "test.rule", Message: "other"},
			ignore: ToolConfigIgnore{Identifier: "test.rule", Message: "missing"},
			pathMatches: func(_, _ string) bool {
				return true
			},
			want: false,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, IgnoreMatches(tt.result, tt.ignore, tt.pathMatches))
		})
	}
}
