package verifier

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestIsPhpStanNoFilesOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "phpstan 2.x error block",
			output: "\n [ERROR] No files found to analyse.                                            \n\n",
			want:   true,
		},
		{
			name:   "phpstan 1.x note block",
			output: "\n ! [NOTE] No files found to analyse.                                            \n\n",
			want:   true,
		},
		{
			name:   "regular json output",
			output: `{"totals":{"errors":0,"file_errors":0},"files":{},"errors":[]}`,
			want:   false,
		},
		{
			name:   "empty output",
			output: "",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isPhpStanNoFilesOutput(tt.output))
		})
	}
}

func TestPhpStanAnalysesAllSourceDirectoriesInOneRun(t *testing.T) {
	rootDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(rootDir, "composer.json"), []byte(`{}`), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(rootDir, "vendor"), 0o755))
	sourceDirectories := []string{filepath.Join(rootDir, "src"), filepath.Join(rootDir, "lib")}

	// A fake php records every run and reports no errors.
	runsLog := filepath.Join(t.TempDir(), "runs.log")
	fakeTool(t, "php", "echo \"$*\" >> '"+runsLog+"'\necho '{\"totals\":{\"errors\":0,\"file_errors\":0},\"files\":[],\"errors\":[]}'\n")

	result := runCheckTool(t, PhpStan{}, ToolConfig{
		RootDir:           rootDir,
		ToolDirectory:     t.TempDir(),
		SourceDirectories: sourceDirectories,
	})
	require.Empty(t, result.Errors)

	runs, err := os.ReadFile(runsLog)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(runs)), "\n")
	require.Len(t, lines, 1)
	for _, sourceDirectory := range sourceDirectories {
		assert.Contains(t, lines[0], sourceDirectory)
	}
}
