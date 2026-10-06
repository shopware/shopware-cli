package deployment

import (
	"bytes"
	"context"
	"io"
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

func phpRestartTestCommands(t *testing.T) (signals, pgrepArgs, self string) {
	t.Helper()
	dir := t.TempDir()
	signals = filepath.Join(dir, "signals")
	pgrepArgs = filepath.Join(dir, "pgrep-args")
	self = filepath.Join(dir, "deployment-pid")
	t.Setenv("PHP_RESTART_TEST_SIGNALS", signals)
	t.Setenv("PHP_RESTART_TEST_PGREP_ARGS", pgrepArgs)
	t.Setenv("PHP_RESTART_TEST_SELF", self)
	t.Setenv("PHP_RESTART_TEST_HOSTNAME", "cloud7-vm576.de-nserver.de")
	t.Setenv("PHP_RESTART_TEST_HOSTNAME_EXIT", "0")
	t.Setenv("PHP_RESTART_TEST_HOSTNAME_ARGS", filepath.Join(dir, "hostname-args"))
	t.Setenv("PHP_RESTART_TEST_UID", "1001")
	t.Setenv("PHP_RESTART_TEST_PGREP_EXIT", "0")
	t.Setenv("PHP_RESTART_TEST_KILL_EXIT", "0")
	t.Setenv("PHP_RESTART_TEST_EXTRA_PID", "")
	t.Setenv("PHP_RESTART_TEST_CURRENT", "")
	t.Setenv("PHP_RESTART_TEST_ACTIVATIONS", filepath.Join(dir, "activations"))
	// Never invoke real pgrep or kill: every signal in these tests is recorded.
	scripts := map[string]string{
		"curl": "#!/bin/sh\nexit 93\n",
		"hostname": `#!/bin/sh
printf '%s\n' "$@" > "$PHP_RESTART_TEST_HOSTNAME_ARGS"
[ "$PHP_RESTART_TEST_HOSTNAME_EXIT" = 0 ] || exit "$PHP_RESTART_TEST_HOSTNAME_EXIT"
printf '%s\n' "$PHP_RESTART_TEST_HOSTNAME"
`,
		"id": `#!/bin/sh
printf '%s\n' "$PHP_RESTART_TEST_UID"
`,
		"pgrep": `#!/bin/sh
printf '%s\n' "$@" > "$PHP_RESTART_TEST_PGREP_ARGS"
[ "$PHP_RESTART_TEST_PGREP_EXIT" = 0 ] || exit "$PHP_RESTART_TEST_PGREP_EXIT"
[ ! -f "$PHP_RESTART_TEST_SELF" ] || cat "$PHP_RESTART_TEST_SELF"
printf '0\n1\n101\n202\n'
[ -z "$PHP_RESTART_TEST_EXTRA_PID" ] || printf '%s\n' "$PHP_RESTART_TEST_EXTRA_PID"
`,
		"kill": `#!/bin/sh
printf '%s\n' "$@" >> "$PHP_RESTART_TEST_SIGNALS"
if [ -n "$PHP_RESTART_TEST_CURRENT" ]; then
    readlink "$PHP_RESTART_TEST_CURRENT" >> "$PHP_RESTART_TEST_ACTIVATIONS"
fi
exit "$PHP_RESTART_TEST_KILL_EXIT"
`,
	}
	for name, content := range scripts {
		filename := filepath.Join(dir, name)
		testhelper.WriteFile(t, filename, content)
		require.NoError(t, os.Chmod(filename, 0o755))
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return signals, pgrepArgs, self
}

func TestPHPProcessRestartScopesAndValidatesPIDs(t *testing.T) {
	for _, scenario := range []string{"signals", "uppercase hostname", "different host", "hostname error", "no matches", "pgrep error", "root", "invalid PID"} {
		t.Run(scenario, func(t *testing.T) {
			php := deploymentPHP(t)
			signals, args, self := phpRestartTestCommands(t)
			switch scenario {
			case "uppercase hostname":
				t.Setenv("PHP_RESTART_TEST_HOSTNAME", "CLOUD7.DE-NSERVER.DE")
			case "different host":
				t.Setenv("PHP_RESTART_TEST_HOSTNAME", "shop.example.com")
			case "hostname error":
				t.Setenv("PHP_RESTART_TEST_HOSTNAME_EXIT", "2")
			case "no matches":
				t.Setenv("PHP_RESTART_TEST_PGREP_EXIT", "1")
			case "pgrep error":
				t.Setenv("PHP_RESTART_TEST_PGREP_EXIT", "2")
			case "root":
				t.Setenv("PHP_RESTART_TEST_UID", "0")
			case "invalid PID":
				t.Setenv("PHP_RESTART_TEST_EXTRA_PID", "-1")
			}
			script := strings.TrimPrefix(sshCachetoolScript, "<?php\n") +
				strings.TrimPrefix(sshPHPRestartScript, "<?php\n") +
				"\nfile_put_contents($argv[1], getmypid() . \"\\n\"); deploymentRestartPHPProcesses(); echo 'survived';"
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			output, err := exec.CommandContext(ctx, php, "-r", script, "--", self).CombinedOutput()
			if scenario == "signals" || scenario == "uppercase hostname" || scenario == "different host" || scenario == "no matches" {
				require.NoError(t, err, string(output))
				assert.Contains(t, string(output), "survived")
			} else {
				require.Error(t, err)
			}
			if scenario == "signals" || scenario == "uppercase hostname" {
				got, err := os.ReadFile(signals)
				require.NoError(t, err)
				assert.Equal(t, "-TERM\n101\n-TERM\n202\n", string(got), "exclude own PID, PID 1 and process-group PID 0")
				filter, err := os.ReadFile(args)
				require.NoError(t, err)
				assert.Equal(t, "-u\n1001\nphp\n", string(filter))
			} else {
				assert.NoFileExists(t, signals)
			}
			if scenario == "root" {
				assert.NoFileExists(t, args)
				assert.Contains(t, string(output), "root account")
			}
			hostnameArgs, err := os.ReadFile(os.Getenv("PHP_RESTART_TEST_HOSTNAME_ARGS"))
			require.NoError(t, err)
			assert.Equal(t, "-f\n", string(hostnameArgs))
			if scenario == "different host" || scenario == "hostname error" {
				assert.NoFileExists(t, args)
			}
		})
	}
}

func TestProviderPHPProcessRestartAfterActivation(t *testing.T) {
	s := localDeploymentSSH(t)
	signals, _, _ := phpRestartTestCommands(t)
	// SSH's configured host is an alias; detection must use remote hostname -f.
	s.env.SSH.Host = "production-alias"
	t.Setenv("PHP_RESTART_TEST_CURRENT", s.directory)
	archive := deploymentTestArchive(t, "")
	for _, name := range []string{"first", "second"} {
		_, err := s.rolloutArchive(t.Context(), Deployment{Reference: name}, archive, io.Discard)
		require.NoError(t, err)
	}
	_, err := s.ActivateDeployment(t.Context(), Deployment{Reference: "first"}, io.Discard)
	require.NoError(t, err)
	before, err := os.ReadFile(signals)
	require.NoError(t, err)
	assert.Equal(t, 6, strings.Count(string(before), "-TERM\n"))
	activeAtSignal, err := os.ReadFile(os.Getenv("PHP_RESTART_TEST_ACTIVATIONS"))
	require.NoError(t, err)
	assert.Equal(t, []string{"releases/first", "releases/first", "releases/second", "releases/second", "releases/first", "releases/first"},
		strings.Split(strings.TrimSpace(string(activeAtSignal)), "\n"))
	result, err := s.ActivateDeployment(t.Context(), Deployment{Reference: "first"}, io.Discard)
	require.NoError(t, err)
	assert.True(t, result.Unchanged)
	after, err := os.ReadFile(signals)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestProviderPHPProcessRestartFailureKeepsActivation(t *testing.T) {
	s := localDeploymentSSH(t)
	phpRestartTestCommands(t)
	s.env.SSH.Host = "production-alias"
	t.Setenv("PHP_RESTART_TEST_KILL_EXIT", "1")
	archive := deploymentTestArchive(t, "")
	var output bytes.Buffer
	result, err := s.RolloutDeployment(t.Context(), Deployment{Reference: archive}, &output)
	require.NoError(t, err)
	assert.True(t, result.Active)
	assert.Contains(t, output.String(), "Deployment activated, but")
	assert.Equal(t, "releases/shop", currentRelease(t, s))
}

func TestProviderPHPProcessRestartWhenCachetoolDisabled(t *testing.T) {
	s := localDeploymentSSH(t)
	signals, _, _ := phpRestartTestCommands(t)
	s.env.SSH.Cachetool = &shop.EnvironmentSSHCachetoolConfig{Enabled: new(false), Adapter: "fcgi"}
	archive := deploymentTestArchive(t, "")
	_, err := s.RolloutDeployment(t.Context(), Deployment{Reference: archive}, io.Discard)
	require.NoError(t, err)
	assert.FileExists(t, signals)
	assert.NoDirExists(t, filepath.Join(filepath.Dir(s.directory), ".shopware-cli/tools"), "disabled CacheTool must not download or execute its PHAR")
}

func TestCachetoolTakesPrecedenceOverPHPProcessRestart(t *testing.T) {
	s := localDeploymentSSH(t)
	signals, _, _ := phpRestartTestCommands(t)
	config, _, _ := cachetoolTestFixture(t, s)
	archive := deploymentTestArchive(t, "")
	input, data := cachetoolTestInput(t, s, archive, "first", "event-first", config)
	// Even contradictory remote input must not run both reset mechanisms.
	input.ProbePHPHost = true
	_, err := s.runSSHDeployment(t.Context(), bytes.NewReader(data), input, io.Discard)
	require.NoError(t, err)
	assert.NoFileExists(t, signals)
	assert.NoFileExists(t, os.Getenv("PHP_RESTART_TEST_HOSTNAME_ARGS"), "enabled CacheTool skips even the hostname probe")
}
