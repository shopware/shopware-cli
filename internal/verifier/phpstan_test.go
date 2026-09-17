package verifier

import (
	"os"
	"path"
	"path/filepath"
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

func TestPhpStan_configArguments(t *testing.T) {
	toolDir := t.TempDir()
	bundledConfig := path.Join(toolDir, "php", "configs", "phpstan.neon")

	tests := []struct {
		name          string
		phpstanConfig string
		rootFiles     []string
		rootDirs      []string
		wantConfig    string
		wantErr       string
	}{
		{
			name:          "extension supplied config is used",
			phpstanConfig: "phpstan-verifier.neon",
			rootFiles:     []string{"phpstan-verifier.neon"},
			wantConfig:    "phpstan-verifier.neon",
		},
		{
			name:          "extension supplied config wins over a discovered one",
			phpstanConfig: "phpstan-verifier.neon",
			rootFiles:     []string{"phpstan-verifier.neon", "phpstan.neon.dist"},
			wantConfig:    "phpstan-verifier.neon",
		},
		{
			name:      "discovered config is left to phpstan",
			rootFiles: []string{"phpstan.neon.dist"},
		},
		{
			name:       "bundled config when the extension has none",
			wantConfig: bundledConfig,
		},
		{
			name:          "unreadable file is reported",
			phpstanConfig: "phpstan-verifier.neon",
			wantErr:       "cannot be read",
		},
		{
			name:          "directory is reported",
			phpstanConfig: "configs",
			rootDirs:      []string{"configs"},
			wantErr:       "is a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootDir := t.TempDir()

			for _, file := range tt.rootFiles {
				require.NoError(t, os.WriteFile(path.Join(rootDir, file), []byte("parameters:\n"), 0o600))
			}

			for _, dir := range tt.rootDirs {
				require.NoError(t, os.Mkdir(path.Join(rootDir, dir), 0o750))
			}

			arguments, err := PhpStan{}.configArguments(ToolConfig{
				ToolDirectory: toolDir,
				RootDir:       rootDir,
				PhpstanConfig: tt.phpstanConfig,
			})

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "validation.phpstan_config")
				assert.Contains(t, err.Error(), tt.wantErr)

				return
			}

			require.NoError(t, err)

			if tt.wantConfig == "" {
				assert.Empty(t, arguments)

				return
			}

			wantConfig := tt.wantConfig

			if !filepath.IsAbs(wantConfig) {
				wantConfig = filepath.Join(rootDir, wantConfig)
			}

			assert.Equal(t, []string{"--configuration", wantConfig}, arguments)
		})
	}
}
