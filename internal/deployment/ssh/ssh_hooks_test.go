package ssh

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	for _, tc := range []struct {
		name        string
		failPre     bool
		failPost    bool
		timeoutPre  bool
		timeoutPost bool
	}{
		{name: "success"},
		{name: "timed out pre", failPre: true, timeoutPre: true},
		{name: "timed out post", failPost: true, timeoutPost: true},
		{name: "timed out cleanup", failPre: true, failPost: true, timeoutPre: true, timeoutPost: true},
		{name: "failed pre", failPre: true},
		{name: "failed post", failPost: true},
		{name: "failed pre and post", failPre: true, failPost: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			t.Cleanup(func() {
				require.NoError(t, exec.CommandContext(context.WithoutCancel(t.Context()), "chmod", "-R", "u+rwX", root).Run())
			})
			require.NoError(t, os.MkdirAll(filepath.Join(root, "releases", "release"), 0755))
			checksum := strings.Repeat("a", 64)
			testhelper.WriteFile(t, filepath.Join(root, ".shopware-cli", "releases", "release.json"), `{"ready":true,"sha256":"`+checksum+`"}`)
			pre := "printf pre >> hooks.log"
			if tc.timeoutPre {
				pre += "; trap '' TERM; sleep 30; printf leaked >> hooks.log"
			} else if tc.failPre {
				pre += "; exit 1"
			}
			post := "printf post >> hooks.log"
			if tc.timeoutPost {
				post += "; sleep 30; printf leaked >> hooks.log"
			} else if tc.failPost {
				post += "; exit 1"
			}
			timeout := 0
			if tc.timeoutPre || tc.timeoutPost {
				timeout = 1
			}
			payload, err := json.Marshal(sshRolloutInput{Size: 1, Root: root, Reference: "event", Release: "release", Deployment: "release", SHA256: checksum, Hooks: &shop.EnvironmentSSHDeploymentHooks{
				TimeoutSeconds: timeout, Pre: []string{pre}, Post: []string{post, post, "printf restart >> hooks.log"},
			}})
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, php, "-r", strings.TrimPrefix(sshDeploymentScript, "<?php\n"), "--", string(payload))
			cmd.Stdin = strings.NewReader("ACTIVATE event\n")
			cmd.WaitDelay = time.Second
			output, err := cmd.CombinedOutput()
			require.NoError(t, ctx.Err(), string(output))
			if tc.failPre || tc.failPost {
				require.Error(t, err, string(output))
				assert.NotContains(t, string(output), "ACTIVE event\n")
			} else {
				require.NoError(t, err, string(output))
				assert.Contains(t, string(output), "ACTIVE event\n")
			}
			if tc.failPre {
				assert.NoFileExists(t, filepath.Join(root, "current"))
			} else {
				link, err := os.Readlink(filepath.Join(root, "current"))
				require.NoError(t, err)
				assert.Equal(t, "releases/release", link)
			}
			data, err := os.ReadFile(filepath.Join(root, "hooks.log"))
			require.NoError(t, err, string(output))
			assert.Equal(t, "prepostpostrestart", string(data))
			// A timed-out child must not retain the deployment lock after cleanup.
			lock := exec.CommandContext(ctx, php, "-r", `$lock = fopen($argv[1], 'r'); exit(flock($lock, LOCK_EX | LOCK_NB) ? 0 : 1);`, filepath.Join(root, ".shopware-cli", "deployment.lock"))
			require.NoError(t, lock.Run())
		})
	}
}
