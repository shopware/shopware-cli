package verifier

import (
	"bytes"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPhpStan_configDefinesPaths(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		content  string
		want     bool
	}{
		{
			name:     "config with paths",
			fileName: "phpstan.neon",
			content:  "parameters:\n    level: 6\n    paths:\n        - custom/static-plugins\n",
			want:     true,
		},
		{
			name:     "config with only excludePaths",
			fileName: "phpstan.neon",
			content:  "parameters:\n    level: 6\n    excludePaths:\n        - vendor\n",
			want:     false,
		},
		{
			name:     "config without paths",
			fileName: "phpstan.neon.dist",
			content:  "parameters:\n    level: 6\n",
			want:     false,
		},
		{
			name:     "no config file",
			fileName: "",
			content:  "",
			want:     false,
		},
	}

	p := PhpStan{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.fileName != "" {
				assert.NoError(t, os.WriteFile(path.Join(dir, tt.fileName), []byte(tt.content), 0o644))
			}

			assert.Equal(t, tt.want, p.configDefinesPaths(dir))
		})
	}
}

func TestPhpStan_isNoFilesToAnalyse(t *testing.T) {
	tests := []struct {
		name   string
		stdout []byte
		stderr string
		want   bool
	}{
		{
			name:   "empty stdout with no files notice",
			stdout: []byte("   \n"),
			stderr: "No files found to analyse.\n",
			want:   true,
		},
		{
			name:   "empty stdout without notice",
			stdout: nil,
			stderr: "Some other fatal error",
			want:   false,
		},
		{
			name:   "stdout contains json output",
			stdout: []byte(`{"totals":{"errors":0,"file_errors":0},"files":{},"errors":[]}`),
			stderr: "No files found to analyse.\n",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			stderr.WriteString(tt.stderr)
			got := isNoFilesToAnalyse(tt.stdout, &stderr)
			assert.Equal(t, tt.want, got)
		})
	}
}

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
