package projectarchive

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v3"
)

// localAdapter is the only Flysystem adapter supported so far.
const localAdapter = "local"

const shopwareVersionParameter = "kernel.shopware_version"

// Directories of the public filesystem that belong to an archive.
var publicDirectories = []string{"media", "thumbnail"}

// privateDirectories selects the whole private filesystem.
var privateDirectories = []string{"."}

// packScript writes a tar of directories below the root ($1) to stdout.
// Symlinks are followed, as SSH deployments link shared directories into each
// release. Missing directories are created, so an unused filesystem exports as
// empty. Compression happens in Go: tar implementations disagree on padding
// compressed streams, which GNU tar rejects.
const packScript = `set -e
mkdir -p "$1"
cd "$1"
shift
mkdir -p "$@"
exec tar -chf - "$@"`

// unpackScript extracts the tar read from stdin into a staging directory
// below the root ($1), then replaces the contents of each directory with the
// extracted one. Extracting first leaves the target untouched when the archive
// is broken. Moving contents keeps directories that are symlinks into a shared
// directory in place, which tar would replace with real directories.
const unpackScript = `set -e
mkdir -p "$1"
cd "$1"
shift
staging=$(mktemp -d .shopware-import.XXXXXX)
trap 'rm -rf "$staging"' EXIT
tar -xf - -C "$staging"
for dir in "$@"; do
	mkdir -p "$dir"
	find "$dir/" -mindepth 1 -maxdepth 1 ! -name "$staging" -exec rm -rf {} \;
	if [ -d "$staging/$dir" ]; then
		find "$staging/$dir/" -mindepth 1 -maxdepth 1 -exec mv {} "$dir/" \;
	fi
done`

// Filesystem is a Shopware filesystem configured in shopware.filesystem.
type Filesystem struct {
	Type   string `yaml:"type"`
	Config struct {
		Root string `yaml:"root"`
	} `yaml:"config"`
}

// Filesystems are the public and private Shopware filesystems of a project.
type Filesystems struct {
	Public  Filesystem `yaml:"public"`
	Private Filesystem `yaml:"private"`
}

// ResolveFilesystems reads the resolved shopware.filesystem configuration of
// the project.
func ResolveFilesystems(ctx context.Context, env Environment) (Filesystems, error) {
	output, err := env.ConsoleCommand(ctx, "debug:config", "shopware", "filesystem", "--no-ansi").Output()
	if err != nil {
		return Filesystems{}, fmt.Errorf("cannot read the filesystem configuration: %w", commandError(err))
	}

	return parseFilesystems(output)
}

// parseFilesystems parses the output of debug:config, which prints a title
// underlined with "=" before the YAML configuration.
func parseFilesystems(output []byte) (Filesystems, error) {
	lines := strings.Split(string(output), "\n")
	for i, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" && strings.Trim(trimmed, "=") == "" {
			lines = lines[i+1:]
			break
		}
	}

	var filesystems Filesystems
	if err := yaml.Unmarshal([]byte(strings.Join(lines, "\n")), &filesystems); err != nil {
		return Filesystems{}, fmt.Errorf("cannot parse the filesystem configuration: %w", err)
	}

	return filesystems, nil
}

// localRoot returns the root of a local filesystem and rejects other adapters.
func localRoot(name string, fs Filesystem) (string, error) {
	if !strings.EqualFold(fs.Type, localAdapter) {
		return "", fmt.Errorf("cannot transfer files of the %s filesystem: adapter %q is not supported, only %q", name, fs.Type, localAdapter)
	}
	if fs.Config.Root == "" || fs.Config.Root == "/" {
		return "", fmt.Errorf("cannot transfer files of the %s filesystem: invalid root %q", name, fs.Config.Root)
	}

	return fs.Config.Root, nil
}

// ShopwareVersion returns the version of the Shopware project.
func ShopwareVersion(ctx context.Context, env Environment) (string, error) {
	output, err := env.ConsoleCommand(ctx, "debug:container", "--parameter="+shopwareVersionParameter, "--format=json", "--no-ansi").Output()
	if err != nil {
		return "", fmt.Errorf("cannot detect the Shopware version: %w", commandError(err))
	}

	return parseShopwareVersion(output)
}

func parseShopwareVersion(output []byte) (string, error) {
	// Deprecation notices may precede the JSON document.
	if start := bytes.IndexByte(output, '{'); start > 0 {
		output = output[start:]
	}

	var parameters map[string]string
	if err := json.Unmarshal(output, &parameters); err != nil {
		return "", fmt.Errorf("cannot detect the Shopware version: %w", err)
	}

	version := parameters[shopwareVersionParameter]
	if version == "" {
		return "", errors.New("cannot detect the Shopware version: parameter " + shopwareVersionParameter + " is empty")
	}

	return version, nil
}

// packFiles writes a tar.gz of the directories below root to w.
func packFiles(ctx context.Context, env Environment, root string, dirs []string, w io.Writer) error {
	process := env.Command(ctx, "sh", append([]string{"-c", packScript, "sh", root}, dirs...)...)
	gz := gzip.NewWriter(w)
	var stderr bytes.Buffer
	process.Cmd.Stdout = gz
	process.Cmd.Stderr = &stderr

	if err := process.Run(); err != nil {
		return withStderr(err, stderr.String())
	}

	return gz.Close()
}

// unpackFiles replaces the directories below root with the tar.gz read from r.
func unpackFiles(ctx context.Context, env Environment, root string, dirs []string, r io.Reader) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	// Stop after the first member, so zero padding behind it is not
	// mistaken for another one.
	gz.Multistream(false)

	process := env.Command(ctx, "sh", append([]string{"-c", unpackScript, "sh", root}, dirs...)...)
	var stderr bytes.Buffer
	process.Cmd.Stdin = gz
	process.Cmd.Stderr = &stderr

	if err := process.Run(); err != nil {
		return withStderr(err, stderr.String())
	}

	return nil
}

// commandError adds the captured stderr of a failed command to its error.
func commandError(err error) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return withStderr(err, string(exitErr.Stderr))
	}

	return err
}

func withStderr(err error, stderr string) error {
	if stderr = strings.TrimSpace(stderr); stderr != "" {
		return fmt.Errorf("%w: %s", err, stderr)
	}

	return err
}
