package ssh

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestHostHooksAroundActivation(t *testing.T) {
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("PHP required")
	}
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failed pre"}[fail], func(t *testing.T) {
			root := t.TempDir()
			t.Cleanup(func() {
				require.NoError(t, exec.CommandContext(context.WithoutCancel(t.Context()), "chmod", "-R", "u+rwX", root).Run())
			})
			require.NoError(t, os.MkdirAll(filepath.Join(root, "releases", "release"), 0755))
			checksum := strings.Repeat("a", 64)
			testhelper.WriteFile(t, filepath.Join(root, ".shopware-cli", "releases", "release.json"), `{"ready":true,"sha256":"`+checksum+`"}`)
			pre := "printf pre >> hooks.log"
			if fail {
				pre += "; exit 1"
			}
			payload, err := json.Marshal(sshRolloutInput{Size: 1, Root: root, Reference: "event", Release: "release", Deployment: "release", SHA256: checksum, Hooks: &shop.EnvironmentSSHDeploymentHooks{
				Pre: []string{pre}, Post: []string{"printf post >> hooks.log"},
			}})
			require.NoError(t, err)
			cmd := exec.CommandContext(t.Context(), php, "-r", strings.TrimPrefix(sshDeploymentScript, "<?php\n"), "--", string(payload))
			cmd.Stdin = strings.NewReader("ACTIVATE event\n")
			output, err := cmd.CombinedOutput()
			if fail {
				require.Error(t, err, string(output))
				assert.NoFileExists(t, filepath.Join(root, "current"))
			} else {
				require.NoError(t, err, string(output))
				link, err := os.Readlink(filepath.Join(root, "current"))
				require.NoError(t, err)
				assert.Equal(t, "releases/release", link)
			}
			data, err := os.ReadFile(filepath.Join(root, "hooks.log"))
			require.NoError(t, err, string(output))
			assert.Equal(t, "prepost", string(data))
		})
	}
}
