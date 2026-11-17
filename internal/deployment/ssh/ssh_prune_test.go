package ssh

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestPruneConfirmedPlan(t *testing.T) {
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("PHP is required")
	}
	for _, change := range []string{"unchanged", "activation", "artifact", "empty preview"} {
		t.Run(change, func(t *testing.T) {
			root := t.TempDir()
			testhelper.WriteFile(t, filepath.Join(root, ".shopware-cli/deployment.lock"), "")
			for _, name := range []string{"A", "B"} {
				require.NoError(t, os.MkdirAll(filepath.Join(root, "releases", name), 0o755))
				record, err := json.Marshal(map[string]any{"reference": name, "status": "successful", "sha256": strings.Repeat("a", 64), "created_at": "2026-11-01T00:00:00Z"})
				require.NoError(t, err)
				testhelper.WriteFile(t, filepath.Join(root, ".shopware-cli/rollouts", name+".json"), string(record))
			}
			require.NoError(t, os.Symlink(filepath.Join(root, "releases/A"), filepath.Join(root, "current")))
			run := func(options DeploymentPruneOptions) (DeploymentPruneResult, error) {
				t.Helper()
				payload, err := json.Marshal(struct {
					Root string `json:"root"`
					DeploymentPruneOptions
				}{root, options})
				require.NoError(t, err)
				output, err := exec.CommandContext(t.Context(), php, "-r", strings.TrimPrefix(withSSHDeploymentLayout(sshDeploymentPruneScript), "<?php\n"), "--", string(payload)).Output()
				var result DeploymentPruneResult
				require.NoError(t, json.Unmarshal(output, &result))
				return result, err
			}
			preview, err := run(DeploymentPruneOptions{DryRun: true})
			require.NoError(t, err)
			require.Equal(t, []Deployment{{Reference: "B"}}, preview.Deployments)
			switch change {
			case "activation":
				require.NoError(t, os.Remove(filepath.Join(root, "current")))
				require.NoError(t, os.Symlink(filepath.Join(root, "releases/B"), filepath.Join(root, "current")))
			case "artifact":
				testhelper.WriteFile(t, filepath.Join(root, ".shopware-cli/artifacts", strings.Repeat("b", 64)+".tar.gz"), "archive")
			case "empty preview":
				preview = DeploymentPruneResult{}
			}
			result, err := run(DeploymentPruneOptions{ExpectedPlan: &preview})
			if change == "unchanged" {
				require.NoError(t, err)
				assert.Equal(t, preview.Deployments, result.Deployments)
				assert.NoDirExists(t, filepath.Join(root, "releases/B"))
			} else {
				require.Error(t, err)
				assert.Empty(t, result.Deployments)
				assert.Empty(t, result.Artifacts)
				assert.DirExists(t, filepath.Join(root, "releases/B"))
				assert.FileExists(t, filepath.Join(root, ".shopware-cli/rollouts/B.json"))
			}
			assert.DirExists(t, filepath.Join(root, "releases/A"))
		})
	}
}
