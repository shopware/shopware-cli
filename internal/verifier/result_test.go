package verifier

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/validation"
)

func TestNewCheck(t *testing.T) {
	t.Parallel()
	check := NewCheck()
	assert.NotNil(t, check)
	assert.Empty(t, check.Results)
}

func TestAddResult(t *testing.T) {
	t.Parallel()
	check := NewCheck()
	result := validation.CheckResult{
		Path:       "test.go",
		Line:       1,
		Message:    "test message",
		Severity:   validation.SeverityError,
		Identifier: "TEST001",
	}

	check.AddResult(result)
	assert.Len(t, check.Results, 1)
	assert.Equal(t, result, check.Results[0])
}

func TestHasErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		results  []validation.CheckResult
		expected bool
	}{
		{
			name:     "no results",
			results:  []validation.CheckResult{},
			expected: false,
		},
		{
			name: "no errors",
			results: []validation.CheckResult{
				{Severity: validation.SeverityWarning},
				{Severity: "info"},
			},
			expected: false,
		},
		{
			name: "has errors",
			results: []validation.CheckResult{
				{Severity: validation.SeverityWarning},
				{Severity: validation.SeverityError},
				{Severity: "info"},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			check := NewCheck()
			for _, result := range tt.results {
				check.AddResult(result)
			}
			assert.Equal(t, tt.expected, check.HasErrors())
		})
	}
}

func TestRemoveByIdentifier(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		initialResults  []validation.CheckResult
		ignores         []validation.ToolConfigIgnore
		expectedResults []validation.CheckResult
	}{
		{
			name: "remove single result",
			initialResults: []validation.CheckResult{
				{Path: "file1.go", Identifier: "TEST001"},
				{Path: "file2.go", Identifier: "TEST002"},
			},
			ignores: []validation.ToolConfigIgnore{
				{Path: "file1.go", Identifier: "TEST001"},
			},
			expectedResults: []validation.CheckResult{
				{Path: "file2.go", Identifier: "TEST002"},
			},
		},
		{
			name: "remove by identifier only",
			initialResults: []validation.CheckResult{
				{Path: "file1.go", Identifier: "TEST001"},
				{Path: "file2.go", Identifier: "TEST001"},
			},
			ignores: []validation.ToolConfigIgnore{
				{Identifier: "TEST001"},
			},
			expectedResults: []validation.CheckResult{},
		},
		{
			name: "no matches",
			initialResults: []validation.CheckResult{
				{Path: "file1.go", Identifier: "TEST001"},
				{Path: "file2.go", Identifier: "TEST002"},
			},
			ignores: []validation.ToolConfigIgnore{
				{Path: "file3.go", Identifier: "TEST003"},
			},
			expectedResults: []validation.CheckResult{
				{Path: "file1.go", Identifier: "TEST001"},
				{Path: "file2.go", Identifier: "TEST002"},
			},
		},
		{
			name: "identifier with message should not ignore all",
			initialResults: []validation.CheckResult{
				{Path: "file1.go", Identifier: "TEST001", Message: "error 1"},
				{Path: "file2.go", Identifier: "TEST001", Message: "error 2"},
			},
			ignores: []validation.ToolConfigIgnore{
				{Identifier: "TEST001", Message: "error 1"},
			},
			expectedResults: []validation.CheckResult{
				{Path: "file1.go", Identifier: "TEST001", Message: "error 1"},
				{Path: "file2.go", Identifier: "TEST001", Message: "error 2"},
			},
		},
		{
			name: "identifier only should ignore all matching errors",
			initialResults: []validation.CheckResult{
				{Path: "file1.go", Identifier: "TEST001", Message: "error 1"},
				{Path: "file2.go", Identifier: "TEST001", Message: "error 2"},
				{Path: "file3.go", Identifier: "TEST002", Message: "error 3"},
			},
			ignores: []validation.ToolConfigIgnore{
				{Identifier: "TEST001"},
			},
			expectedResults: []validation.CheckResult{
				{Path: "file3.go", Identifier: "TEST002", Message: "error 3"},
			},
		},
		{
			name: "multiple ignores with different conditions",
			initialResults: []validation.CheckResult{
				{Path: "file1.go", Identifier: "TEST001", Message: "error 1"},
				{Path: "file2.go", Identifier: "TEST001", Message: "error 2"},
				{Path: "file3.go", Identifier: "TEST002", Message: "error 3"},
				{Path: "file4.go", Identifier: "TEST003", Message: "error 4"},
			},
			ignores: []validation.ToolConfigIgnore{
				{Identifier: "TEST001"},                   // Should remove all TEST001
				{Path: "file3.go", Identifier: "TEST002"}, // Should remove only TEST002 in file3.go
				{Message: "error 4"},                      // Should remove anything with this message
			},
			expectedResults: []validation.CheckResult{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			check := NewCheck()
			for _, result := range tt.initialResults {
				check.AddResult(result)
			}

			check.RemoveByIdentifier(tt.ignores)
			assert.ElementsMatch(t, tt.expectedResults, check.Results)
		})
	}
}

func TestRemoveByMessage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		initialResults  []validation.CheckResult
		ignores         []validation.ToolConfigIgnore
		expectedResults []validation.CheckResult
	}{
		{
			name: "remove_single_result_by_exact_message",
			initialResults: []validation.CheckResult{
				{Path: "file1.go", Line: 1, Message: "test error message", Severity: validation.SeverityError},
				{Path: "file2.go", Line: 2, Message: "another error message", Severity: validation.SeverityError},
			},
			ignores: []validation.ToolConfigIgnore{
				{Path: "file1.go", Message: "test error message"},
			},
			expectedResults: []validation.CheckResult{
				{Path: "file2.go", Line: 2, Message: "another error message", Severity: validation.SeverityError},
			},
		},
		{
			name: "remove_by_message_only",
			initialResults: []validation.CheckResult{
				{Path: "file1.go", Line: 1, Message: "test error message", Severity: validation.SeverityError},
				{Path: "file2.go", Line: 2, Message: "test error message", Severity: validation.SeverityError},
			},
			ignores: []validation.ToolConfigIgnore{
				{Message: "test error message"},
			},
			expectedResults: []validation.CheckResult{},
		},
		{
			name: "remove_by_partial_message_match",
			initialResults: []validation.CheckResult{
				{Path: "file1.go", Line: 1, Message: "test error message", Severity: validation.SeverityError},
				{Path: "file2.go", Line: 2, Message: "another error message", Severity: validation.SeverityError},
			},
			ignores: []validation.ToolConfigIgnore{
				{Message: "test error"},
			},
			expectedResults: []validation.CheckResult{
				{Path: "file2.go", Line: 2, Message: "another error message", Severity: validation.SeverityError},
			},
		},
		{
			name: "no_matches",
			initialResults: []validation.CheckResult{
				{Path: "file1.go", Line: 1, Message: "test error message", Severity: validation.SeverityError},
				{Path: "file2.go", Line: 2, Message: "another error message", Severity: validation.SeverityError},
			},
			ignores: []validation.ToolConfigIgnore{
				{Message: "non-existent message"},
			},
			expectedResults: []validation.CheckResult{
				{Path: "file1.go", Line: 1, Message: "test error message", Severity: validation.SeverityError},
				{Path: "file2.go", Line: 2, Message: "another error message", Severity: validation.SeverityError},
			},
		},
		{
			name: "remove_by_path_and_partial_message",
			initialResults: []validation.CheckResult{
				{Path: "file1.go", Line: 1, Message: "test error message", Severity: validation.SeverityError},
				{Path: "file2.go", Line: 2, Message: "test error message", Severity: validation.SeverityError},
			},
			ignores: []validation.ToolConfigIgnore{
				{Path: "file1.go", Message: "test"},
			},
			expectedResults: []validation.CheckResult{
				{Path: "file2.go", Line: 2, Message: "test error message", Severity: validation.SeverityError},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			check := NewCheck()
			for _, result := range tt.initialResults {
				check.AddResult(result)
			}

			check.RemoveByIdentifier(tt.ignores)
			assert.ElementsMatch(t, tt.expectedResults, check.Results)
		})
	}
}
