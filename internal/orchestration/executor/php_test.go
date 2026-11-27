package executor_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/orchestration/executor"
	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

func TestPHPCommandsUseExecutorDefaults(t *testing.T) {
	runner := executor.New(runtimeFunc(nil)).WithDir("/workspace").WithEnv(map[string]string{"BASE": "base"})

	for _, tt := range []struct {
		name    string
		command *executor.Command
		want    runtime.Command
	}{
		{
			name:    "php",
			command: runner.PHP().Command("-v"),
			want:    runtime.Command{Name: "php", Args: []string{"-v"}},
		},
		{
			name:    "composer",
			command: runner.PHP().Composer("install"),
			want:    runtime.Command{Name: "composer", Args: []string{"install"}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.want.Dir = "/workspace"
			tt.want.Env = map[string]string{"BASE": "base"}
			assert.Equal(t, tt.want, tt.command.Description())
		})
	}
}
