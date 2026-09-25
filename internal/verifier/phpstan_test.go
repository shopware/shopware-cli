package verifier

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/validation"
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

func TestPhpStanSkipsWithoutComposerJSON(t *testing.T) {
	t.Parallel()
	check := NewCheck()

	require.NoError(t, PhpStan{}.Check(t.Context(), check, ToolConfig{RootDir: t.TempDir()}))

	assert.Equal(t, []validation.ToolRun{{Name: "phpstan", Status: validation.ToolRunSkipped, Note: "no composer.json"}}, check.GetToolRuns())
}

func TestPinnedTarget(t *testing.T) {
	t.Parallel()
	flag := validation.Target{Version: "6.7.14.2", Source: validation.TargetSourceFlag}
	derived := validation.Target{Version: "6.7.14.2", Source: validation.TargetSourceConstraint}
	ext := extension.PlatformPlugin{}

	assert.Equal(t, "6.7.14.2", pinnedTarget(ToolConfig{Target: flag, RootDirIsCopy: true, Extension: ext}))
	assert.Empty(t, pinnedTarget(ToolConfig{Target: flag, RootDirIsCopy: false, Extension: ext}), "never touch the user's directory")
	assert.Empty(t, pinnedTarget(ToolConfig{Target: flag, RootDirIsCopy: true}), "projects keep their own dependencies")
	assert.Empty(t, pinnedTarget(ToolConfig{Target: derived, RootDirIsCopy: true, Extension: ext}), "only an explicit target is installed")
}

func TestPhpstanToolRun(t *testing.T) {
	t.Parallel()
	ext := extension.PlatformPlugin{}
	flag := validation.Target{Version: "6.7.14.2", Source: validation.TargetSourceFlag}
	derived := validation.Target{Version: "6.6.0.0", Source: validation.TargetSourceConstraint}

	cases := map[string]struct {
		config    ToolConfig
		installed string
		want      validation.ToolRun
	}{
		"matches the target": {
			config:    ToolConfig{Target: flag, Extension: ext, RootDirIsCopy: true},
			installed: "6.7.14.2",
			want:      validation.ToolRun{Name: "phpstan", Status: validation.ToolRunRan, Baseline: "6.7.14.2"},
		},
		"matches the target in a different spelling": {
			config:    ToolConfig{Target: validation.Target{Version: "6.7.0.0-RC1", Source: validation.TargetSourceFlag}, Extension: ext, RootDirIsCopy: true},
			installed: "6.7.0.0-rc1",
			want:      validation.ToolRun{Name: "phpstan", Status: validation.ToolRunRan, Baseline: "6.7.0.0-rc1"},
		},
		"unknown installed version": {
			config: ToolConfig{Target: flag, Extension: ext},
			want:   validation.ToolRun{Name: "phpstan", Status: validation.ToolRunRan},
		},
		"extension without flag": {
			config:    ToolConfig{Target: derived, Extension: ext, CheckAgainst: "highest", RootDirIsCopy: true},
			installed: "6.7.14.2",
			want:      validation.ToolRun{Name: "phpstan", Status: validation.ToolRunRan, Baseline: "6.7.14.2", Note: "analysed the installed shopware/core 6.7.14.2; pass --target-version to align all checks"},
		},
		"extension with flag and --no-copy": {
			config:    ToolConfig{Target: flag, Extension: ext},
			installed: "6.6.10.21",
			want:      validation.ToolRun{Name: "phpstan", Status: validation.ToolRunRan, Baseline: "6.6.10.21", Note: "analysed the installed shopware/core 6.6.10.21; drop --no-copy so 6.7.14.2 can be installed in a temporary copy"},
		},
		"extension with flag but a bundled vendor directory": {
			config:    ToolConfig{Target: flag, Extension: ext, RootDirIsCopy: true},
			installed: "6.6.10.21",
			want:      validation.ToolRun{Name: "phpstan", Status: validation.ToolRunRan, Baseline: "6.6.10.21", Note: "analysed the installed shopware/core 6.6.10.21 instead of 6.7.14.2, because the extension ships its own vendor directory"},
		},
		"project without flag": {
			config:    ToolConfig{Target: derived, RootDirIsCopy: true},
			installed: "6.7.9999999-dev",
			want:      validation.ToolRun{Name: "phpstan", Status: validation.ToolRunRan, Baseline: "6.7.9999999-dev", Note: "analysed the installed shopware/core 6.7.9999999-dev; validation does not change project dependencies, so install 6.6.0.0 in the project to include PHPStan"},
		},
		"project with flag": {
			config:    ToolConfig{Target: flag, RootDirIsCopy: true},
			installed: "6.6.10.21",
			want:      validation.ToolRun{Name: "phpstan", Status: validation.ToolRunRan, Baseline: "6.6.10.21", Note: "analysed the installed shopware/core 6.6.10.21; validation does not change project dependencies, so install 6.7.14.2 in the project to include PHPStan"},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, phpstanToolRun(tc.config, tc.installed))
		})
	}
}
