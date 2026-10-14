package deployment

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop"
)

func fixtureInitializationGroup(t *testing.T) (*sshGroup, string) {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
for arg do
  case "$arg" in a.invalid) host=a;; b.invalid) host=b;; esac
done
[ -n "$host" ] || exit 90
payload=$(cat)
printf '%s\n' "$@" >> "$INIT_FIXTURE/$host.args"
case "$payload" in
  *'"action":"apply"'*)
    printf '%s' "$payload" > "$INIT_FIXTURE/$host.apply"
    [ ! -f "$INIT_FIXTURE/$host.fail-apply" ] || exit 91
    printf '{"status":"initialized"}'
    ;;
  *)
    phase=before
    [ ! -f "$INIT_FIXTURE/b.apply" ] || phase=after
    [ ! -f "$INIT_FIXTURE/$host.$phase.fail" ] || exit 92
    cat "$INIT_FIXTURE/$host.$phase.json"
    ;;
esac
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ssh"), []byte(script), 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("INIT_FIXTURE", dir)
	var hosts []sshHost
	for _, name := range []string{"a", "b"} {
		backend, err := New(t.TempDir(), "", &shop.Config{}, &shop.EnvironmentConfig{
			Type: "ssh",
			SSH:  &shop.EnvironmentSSHConfig{Host: name + ".invalid", Directory: "/srv/shop/current"},
		})
		require.NoError(t, err)
		hosts = append(hosts, sshHost{name: name, backend: backend.(*SSH)})
		for _, phase := range []string{"before", "after"} {
			writeInitializationState(t, dir, name, phase, sshDeploymentInitState{
				HasSharedDirectory: true, HasRuntimeConfig: true, RuntimeSHA256: "same",
				RuntimeKeys: []string{"APP_SECRET"},
			})
		}
	}
	g, err := newSSHGroup(hosts, "b", 2)
	require.NoError(t, err)
	return g, dir
}

func writeInitializationState(t *testing.T, dir, host, phase string, state sshDeploymentInitState) {
	t.Helper()
	data, err := json.Marshal(state)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, host+"."+phase+".json"), data, 0o600))
}

func TestSSHGroupInitializationWritesOnceOnMigrationHost(t *testing.T) {
	g, dir := fixtureInitializationGroup(t)
	writeInitializationState(t, dir, "a", "before", sshDeploymentInitState{HasSharedDirectory: true, HasCurrent: true})
	calls := 0
	err := g.initializeDeployment(t.Context(), func(_ context.Context, state sshDeploymentInitState) (sshDeploymentInitConfig, bool, error) {
		calls++
		assert.True(t, state.HasCurrent, "current must be aggregated across hosts")
		assert.True(t, state.HasRuntimeConfig, "runtime state must come from migration host")
		assert.Contains(t, state.RuntimeKeys, "APP_SECRET")
		return sshDeploymentInitConfig{
			RuntimeValues: map[string]string{"APP_SECRET": "one-secret"},
			InstallValues: map[string]string{"INSTALL_ADMIN_PASSWORD": "one-password"},
		}, true, nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	assert.NoFileExists(t, filepath.Join(dir, "a.apply"))
	data, err := os.ReadFile(filepath.Join(dir, "b.apply"))
	require.NoError(t, err)
	var input sshDeploymentInitInput
	require.NoError(t, json.Unmarshal(data, &input))
	assert.Equal(t, "one-secret", input.RuntimeValues["APP_SECRET"])
	assert.Equal(t, "one-password", input.InstallValues["INSTALL_ADMIN_PASSWORD"])
	for _, host := range []string{"a", "b"} {
		args, err := os.ReadFile(filepath.Join(dir, host+".args"))
		require.NoError(t, err)
		assert.NotContains(t, string(args), "one-secret")
		assert.NotContains(t, string(args), "one-password")
	}
}

func TestSSHGroupInitializationCancellation(t *testing.T) {
	g, dir := fixtureInitializationGroup(t)
	calls := 0
	require.NoError(t, g.initializeDeployment(t.Context(), func(context.Context, sshDeploymentInitState) (sshDeploymentInitConfig, bool, error) {
		calls++
		return sshDeploymentInitConfig{RuntimeValues: map[string]string{"APP_SECRET": "not-written"}}, false, nil
	}))
	assert.Equal(t, 1, calls)
	assert.NoFileExists(t, filepath.Join(dir, "a.apply"))
	assert.NoFileExists(t, filepath.Join(dir, "b.apply"))
}

func TestSSHGroupInitializationPreflightCapabilityAndReachability(t *testing.T) {
	for name, unavailable := range map[string]bool{"missing capability": false, "unreachable": true} {
		t.Run(name, func(t *testing.T) {
			g, dir := fixtureInitializationGroup(t)
			if unavailable {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "a.before.fail"), nil, 0o600))
			} else {
				files := []string{"install.lock"}
				g.hosts[0].backend.env.SSH.Shared = &shop.EnvironmentSSHSharedConfig{Files: &files}
			}
			err := g.initializeDeployment(t.Context(), func(context.Context, sshDeploymentInitState) (sshDeploymentInitConfig, bool, error) {
				t.Fatal("preflight must finish before prompting")
				return sshDeploymentInitConfig{}, false, nil
			})
			require.ErrorContains(t, err, "host a:")
			assert.NoFileExists(t, filepath.Join(dir, "a.apply"))
			assert.NoFileExists(t, filepath.Join(dir, "b.apply"))
		})
	}
}

func TestSSHGroupInitializationPromptError(t *testing.T) {
	g, dir := fixtureInitializationGroup(t)
	promptErr := errors.New("prompt failed")
	err := g.initializeDeployment(t.Context(), func(context.Context, sshDeploymentInitState) (sshDeploymentInitConfig, bool, error) {
		return sshDeploymentInitConfig{}, false, promptErr
	})
	require.ErrorIs(t, err, promptErr)
	assert.NoFileExists(t, filepath.Join(dir, "a.apply"))
	assert.NoFileExists(t, filepath.Join(dir, "b.apply"))
}

func TestSSHGroupInitializationApplyError(t *testing.T) {
	g, dir := fixtureInitializationGroup(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.fail-apply"), nil, 0o600))
	err := g.initializeDeployment(t.Context(), func(context.Context, sshDeploymentInitState) (sshDeploymentInitConfig, bool, error) {
		return sshDeploymentInitConfig{RuntimeValues: map[string]string{"APP_ENV": "prod"}}, true, nil
	})
	require.ErrorContains(t, err, "host b:")
	assert.NoFileExists(t, filepath.Join(dir, "a.apply"))
}

func TestSSHGroupInitializationNoChangeStillVerifies(t *testing.T) {
	g, dir := fixtureInitializationGroup(t)
	writeInitializationState(t, dir, "a", "before", sshDeploymentInitState{
		HasSharedDirectory: true, HasRuntimeConfig: true, RuntimeSHA256: "different",
	})
	err := g.initializeDeployment(t.Context(), func(context.Context, sshDeploymentInitState) (sshDeploymentInitConfig, bool, error) {
		return sshDeploymentInitConfig{}, true, nil
	})
	require.ErrorContains(t, err, "host a:")
	assert.ErrorContains(t, err, "differs")
	assert.NoFileExists(t, filepath.Join(dir, "a.apply"))
	assert.NoFileExists(t, filepath.Join(dir, "b.apply"))
}

func TestSSHGroupInitializationPreflight(t *testing.T) {
	for _, test := range []struct {
		name  string
		state sshDeploymentInitState
		want  string
	}{
		{"missing shared directory", sshDeploymentInitState{}, "provision the shared directory"},
		{"stale install", sshDeploymentInitState{HasSharedDirectory: true, HasInstallConfig: true}, "install.env"},
	} {
		t.Run(test.name, func(t *testing.T) {
			g, dir := fixtureInitializationGroup(t)
			writeInitializationState(t, dir, "a", "before", test.state)
			err := g.initializeDeployment(t.Context(), func(context.Context, sshDeploymentInitState) (sshDeploymentInitConfig, bool, error) {
				t.Fatal("prompt must not run after failed preflight")
				return sshDeploymentInitConfig{}, false, nil
			})
			require.ErrorContains(t, err, "host a:")
			assert.ErrorContains(t, err, test.want)
			assert.NoFileExists(t, filepath.Join(dir, "b.apply"))
		})
	}
}

func TestSSHGroupInitializationVerification(t *testing.T) {
	for _, test := range []struct {
		name        string
		state       sshDeploymentInitState
		unreachable bool
		want        string
	}{
		{"different", sshDeploymentInitState{HasRuntimeConfig: true, RuntimeSHA256: "different"}, false, "differs"},
		{"missing", sshDeploymentInitState{}, false, "not visible"},
		{"unreachable", sshDeploymentInitState{}, true, "initialize SSH deployment"},
		{"stale install", sshDeploymentInitState{HasRuntimeConfig: true, RuntimeSHA256: "same", HasInstallConfig: true}, false, "install.env"},
	} {
		t.Run(test.name, func(t *testing.T) {
			g, dir := fixtureInitializationGroup(t)
			writeInitializationState(t, dir, "a", "after", test.state)
			if test.unreachable {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "a.after.fail"), nil, 0o600))
			}
			err := g.initializeDeployment(t.Context(), func(context.Context, sshDeploymentInitState) (sshDeploymentInitConfig, bool, error) {
				return sshDeploymentInitConfig{RuntimeValues: map[string]string{"APP_ENV": "prod"}}, true, nil
			})
			require.ErrorContains(t, err, "host a:")
			assert.ErrorContains(t, err, test.want)
			assert.FileExists(t, filepath.Join(dir, "b.apply"), "verification does not roll back published config")
			assert.NoFileExists(t, filepath.Join(dir, "a.apply"))
		})
	}
}
