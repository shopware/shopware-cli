package deployment

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestSSHCachetoolConfiguration(t *testing.T) {
	s := &SSH{}
	input, err := s.cachetoolInput()
	require.NoError(t, err)
	assert.Nil(t, input)
	s.env = &shop.EnvironmentConfig{SSH: &shop.EnvironmentSSHConfig{
		Cachetool: &shop.EnvironmentSSHCachetoolConfig{},
	}}
	input, err = s.cachetoolInput()
	require.NoError(t, err)
	assert.Nil(t, input)
	s.env.SSH.Cachetool = &shop.EnvironmentSSHCachetoolConfig{Enabled: new(true), Adapter: "fcgi", FCGI: "/run/php/php-fpm.sock"}
	input, err = s.cachetoolInput()
	require.NoError(t, err)
	require.NotNil(t, input)
	assert.Equal(t, cachetoolVersion, input.Version)
	assert.Equal(t, cachetoolURL, input.URL)
	assert.Equal(t, cachetoolSHA256, input.SHA256)
	assert.Equal(t, 30, input.TimeoutSeconds)
	assert.Equal(t, *s.env.SSH.Cachetool, input.EnvironmentSSHCachetoolConfig)
	data, err := json.Marshal(input)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"fcgi":"/run/php/php-fpm.sock"`)
	assert.Contains(t, string(data), `"adapter":"fcgi"`)
	s.env.SSH.Cachetool.Adapter = "cli"
	_, err = s.cachetoolInput()
	require.ErrorContains(t, err, "ssh.cachetool")
}

type cachetoolTestRecord struct {
	Args    []string       `json:"args"`
	Config  map[string]any `json:"config"`
	Current string         `json:"current"`
}

func cachetoolTestFixture(t *testing.T, s *SSH) (*sshCachetoolInput, string, string) {
	t.Helper()
	dir := t.TempDir()
	fixture := filepath.Join(dir, "cachetool.phar")
	records := filepath.Join(dir, "records.jsonl")
	downloads := filepath.Join(dir, "downloads")
	t.Setenv("CACHETOOL_TEST_RECORDS", records)
	t.Setenv("CACHETOOL_TEST_CURRENT", s.directory)
	t.Setenv("CACHETOOL_TEST_FIXTURE", fixture)
	t.Setenv("CACHETOOL_TEST_DOWNLOADS", downloads)
	t.Setenv("CACHETOOL_TEST_FAIL", "")
	t.Setenv("CACHETOOL_TEST_SLEEP", "")
	t.Setenv("CACHETOOL_TEST_DOWNLOAD_FAIL", "")
	// Stub CacheTool: no downloads, FPM connections, or HTTP requests.
	program := `<?php
$index = array_search('--config', $argv, true);
if ($index === false) { exit(91); }
$config = json_decode(file_get_contents($argv[$index + 1]), true);
file_put_contents(getenv('CACHETOOL_TEST_RECORDS'), json_encode([
    'args' => array_slice($argv, 1),
    'config' => $config,
    'current' => realpath(getenv('CACHETOOL_TEST_CURRENT')),
]) . "\n", FILE_APPEND);
if (isset($config['webPath'])) {
    file_put_contents($config['webPath'] . '/cachetool-test.php', '<?php echo "temporary";');
}
fwrite(STDOUT, "cachetool stdout\n");
fwrite(STDERR, "cachetool stderr\n");
if (getenv('CACHETOOL_TEST_SLEEP')) { sleep(10); }
if (getenv('CACHETOOL_TEST_FAIL')) { exit(42); }
`
	testhelper.WriteFile(t, fixture, program)
	sum := sha256.Sum256([]byte(program))
	testhelper.WriteFile(t, filepath.Join(dir, "curl"), `#!/bin/sh
printf 'download\n' >> "$CACHETOOL_TEST_DOWNLOADS"
[ -z "$CACHETOOL_TEST_DOWNLOAD_FAIL" ] || exit 17
output=
while [ "$#" -gt 0 ]; do
    case "$1" in
        --output|-o) output="$2"; shift 2 ;;
        *) shift ;;
    esac
done
[ -n "$output" ] || exit 92
cp "$CACHETOOL_TEST_FIXTURE" "$output"
`)
	require.NoError(t, os.Chmod(filepath.Join(dir, "curl"), 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return &sshCachetoolInput{
		EnvironmentSSHCachetoolConfig: shop.EnvironmentSSHCachetoolConfig{
			Enabled: new(true), Adapter: "fcgi", FCGI: "/run/php/php-fpm.sock",
		},
		Version: cachetoolVersion, URL: cachetoolURL, SHA256: hex.EncodeToString(sum[:]), TimeoutSeconds: 3,
	}, records, downloads
}

func cachetoolTestInput(t *testing.T, s *SSH, archive, name, reference string, config *sshCachetoolInput) (sshRolloutInput, []byte) {
	t.Helper()
	data, err := os.ReadFile(archive)
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	files, directories, err := s.env.SSH.SharedPaths()
	require.NoError(t, err)
	return sshRolloutInput{
		Root: filepath.Dir(s.directory), Release: name, Reference: reference,
		Deployment: name, Archive: archive, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(data)),
		PHP: s.env.SSH.PHPBinary, SharedFiles: files, SharedDirectories: directories, Cachetool: config,
	}, data
}

func cachetoolTestRecords(t *testing.T, file string) []cachetoolTestRecord {
	t.Helper()
	data, err := os.ReadFile(file)
	require.NoError(t, err)
	var records []cachetoolTestRecord
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var record cachetoolTestRecord
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		records = append(records, record)
	}
	return records
}

func assertCachetoolTemporaryFilesRemoved(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, ".shopware-cli/tools"))
	require.NoError(t, err)
	require.Len(t, entries, 1, "temporary downloads/configurations must be removed")
	assert.Equal(t, "cachetool-"+cachetoolVersion+".phar", entries[0].Name())
}

func TestSSHCachetoolAdapters(t *testing.T) {
	for _, mode := range []string{"fcgi", "web", "native"} {
		t.Run(mode, func(t *testing.T) {
			s := localDeploymentSSH(t)
			root := filepath.Dir(s.directory)
			config, records, _ := cachetoolTestFixture(t, s)
			switch mode {
			case "fcgi":
				config.FCGIChroot = "/srv/chroot"
				config.TmpDir = "/srv/chroot/tmp"
			case "web":
				config.Adapter = "web"
				config.FCGI = ""
				config.WebURL = "https://shop.example.com/base"
			case "native":
				config.Adapter = ""
				config.FCGI = ""
				config.Config = filepath.Join(root, "shared/cachetool.yaml")
				testhelper.WriteFile(t, config.Config, "adapter: fastcgi\nfastcgi: /run/php/native.sock\n")
			}
			archive := deploymentTestArchive(t, "")
			input, data := cachetoolTestInput(t, s, archive, "first", "event-first", config)
			var output bytes.Buffer
			result, err := s.runSSHDeployment(t.Context(), bytes.NewReader(data), input, &output)
			require.NoError(t, err, output.String())
			assert.True(t, result.Active)
			assert.Contains(t, output.String(), "cachetool stdout")
			assert.Contains(t, output.String(), "cachetool stderr")
			got := cachetoolTestRecords(t, records)
			require.Len(t, got, 1)
			assert.Equal(t, filepath.Join(root, "releases/first"), got[0].Current, "reset must happen after current switches")
			assert.Contains(t, got[0].Args, "opcache:reset")
			assert.Contains(t, got[0].Args, "--no-interaction")
			assert.Contains(t, got[0].Args, "--no-ansi")
			switch mode {
			case "fcgi":
				assert.Equal(t, "fastcgi", got[0].Config["adapter"])
				assert.Equal(t, config.FCGI, got[0].Config["fastcgi"])
				assert.Equal(t, config.FCGIChroot, got[0].Config["fastcgiChroot"])
				assert.Equal(t, config.TmpDir, got[0].Config["temp_dir"])
			case "web":
				assert.Equal(t, "web", got[0].Config["adapter"])
				assert.Equal(t, "SymfonyHttpClient", got[0].Config["webClient"])
				webPath, ok := got[0].Config["webPath"].(string)
				require.True(t, ok)
				assert.Equal(t, filepath.Join(root, "releases/first/public"), filepath.Dir(webPath))
				assert.Equal(t, config.WebURL+"/"+filepath.Base(webPath), got[0].Config["webUrl"])
				assert.NoDirExists(t, webPath)
			case "native":
				assert.Contains(t, got[0].Args, config.Config)
				assert.FileExists(t, config.Config, "never remove user-managed native configuration")
			}
			assertCachetoolTemporaryFilesRemoved(t, root)
			for path, mode := range map[string]os.FileMode{
				filepath.Join(root, ".shopware-cli/tools"):                                     0o700,
				filepath.Join(root, ".shopware-cli/tools/cachetool-"+cachetoolVersion+".phar"): 0o600,
			} {
				info, err := os.Stat(path)
				require.NoError(t, err)
				assert.Equal(t, mode, info.Mode().Perm())
			}
		})
	}
}

func TestSSHCachetoolReuseAndNoop(t *testing.T) {
	s := localDeploymentSSH(t)
	root := filepath.Dir(s.directory)
	config, records, downloads := cachetoolTestFixture(t, s)
	archive := deploymentTestArchive(t, "")
	for _, name := range []string{"first", "second"} {
		input, data := cachetoolTestInput(t, s, archive, name, "event-"+name, config)
		_, err := s.runSSHDeployment(t.Context(), bytes.NewReader(data), input, nil)
		require.NoError(t, err)
	}
	downloadLog, err := os.ReadFile(downloads)
	require.NoError(t, err)
	assert.Equal(t, "download\n", string(downloadLog), "verified PHAR should be reused")
	require.NoError(t, os.Remove(archive))
	require.NoError(t, os.RemoveAll(filepath.Join(root, ".shopware-cli/artifacts")))
	// A corrupt tool must be replaced and verified before remote-only activation.
	testhelper.WriteFile(t, filepath.Join(root, ".shopware-cli/tools/cachetool-"+cachetoolVersion+".phar"), "corrupt")
	input := sshRolloutInput{Action: "activate", Root: root, Reference: "rollback-event", Release: "first", Deployment: "first", Cachetool: config}
	_, err = s.runSSHDeployment(t.Context(), nil, input, nil)
	require.NoError(t, err)
	got := cachetoolTestRecords(t, records)
	require.Len(t, got, 3)
	assert.Equal(t, filepath.Join(root, "releases/first"), got[2].Current)
	downloadLog, err = os.ReadFile(downloads)
	require.NoError(t, err)
	assert.Equal(t, 2, strings.Count(string(downloadLog), "download\n"))
	input.Reference = "no-op-event"
	unchanged, err := s.runSSHDeployment(t.Context(), nil, input, nil)
	require.NoError(t, err)
	assert.True(t, unchanged.Unchanged)
	assert.Len(t, cachetoolTestRecords(t, records), 3)
	assertCachetoolTemporaryFilesRemoved(t, root)
}

func TestSSHCachetoolPreparationFailureKeepsCurrent(t *testing.T) {
	for _, failure := range []string{"checksum", "download", "symlink", "missing config"} {
		t.Run(failure, func(t *testing.T) {
			s := localDeploymentSSH(t)
			root := filepath.Dir(s.directory)
			archive := deploymentTestArchive(t, "")
			_, err := s.rolloutArchive(t.Context(), Deployment{Reference: "first"}, archive, nil)
			require.NoError(t, err)
			config, records, _ := cachetoolTestFixture(t, s)
			switch failure {
			case "checksum":
				config.SHA256 = strings.Repeat("0", 64)
			case "download":
				t.Setenv("CACHETOOL_TEST_DOWNLOAD_FAIL", "1")
			case "symlink":
				require.NoError(t, os.MkdirAll(filepath.Join(root, ".shopware-cli/tools"), 0o700))
				require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(root, ".shopware-cli/tools/cachetool-"+cachetoolVersion+".phar")))
			case "missing config":
				config.Adapter, config.FCGI = "", ""
				config.Config = filepath.Join(root, "shared/missing.yaml")
			}
			input, data := cachetoolTestInput(t, s, archive, "second", "event-second", config)
			_, err = s.runSSHDeployment(t.Context(), bytes.NewReader(data), input, nil)
			require.Error(t, err)
			assert.Equal(t, "releases/first", currentRelease(t, s))
			assert.NoFileExists(t, records, "never execute an unverified PHAR")
			assert.NoDirExists(t, filepath.Join(root, "releases/second"))
		})
	}
}

func TestSSHCachetoolResetFailureDoesNotUndoActivation(t *testing.T) {
	for _, failure := range []string{"exit", "timeout"} {
		t.Run(failure, func(t *testing.T) {
			s := localDeploymentSSH(t)
			root := filepath.Dir(s.directory)
			config, records, _ := cachetoolTestFixture(t, s)
			config.Adapter, config.FCGI = "web", ""
			config.WebURL = "https://shop.example.com"
			if failure == "timeout" {
				config.TimeoutSeconds = 1
				t.Setenv("CACHETOOL_TEST_SLEEP", "1")
			} else {
				t.Setenv("CACHETOOL_TEST_FAIL", "1")
			}
			archive := deploymentTestArchive(t, "")
			input, data := cachetoolTestInput(t, s, archive, "first", "event-first", config)
			var output bytes.Buffer
			start := time.Now()
			result, err := s.runSSHDeployment(t.Context(), bytes.NewReader(data), input, &output)
			require.NoError(t, err)
			assert.Less(t, time.Since(start), 8*time.Second, "reset timeout must be bounded")
			assert.True(t, result.Active)
			assert.Equal(t, "releases/first", currentRelease(t, s))
			assert.Contains(t, output.String(), "Deployment activated, but OPcache reset failed")
			history, err := s.ListRollouts(t.Context())
			require.NoError(t, err)
			require.Len(t, history, 1)
			assert.True(t, history[0].Active)
			assertCachetoolTemporaryFilesRemoved(t, root)
			got := cachetoolTestRecords(t, records)
			require.Len(t, got, 1)
			webPath, ok := got[0].Config["webPath"].(string)
			require.True(t, ok)
			assert.NoDirExists(t, webPath, "CacheTool endpoint directory must be removed even after timeout")
			entries, err := os.ReadDir(filepath.Join(root, "releases/first/public"))
			require.NoError(t, err)
			for _, entry := range entries {
				assert.False(t, strings.Contains(entry.Name(), "cachetool-"), "public CacheTool files must be cleaned on failure")
			}
		})
	}
}
