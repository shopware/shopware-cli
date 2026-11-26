package projectarchive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
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

// unpackScript extracts the tar read from stdin into a staging directory, then
// replaces the contents of each directory below the root ($1) with the
// extracted one. It does not extract directly into the directories:
//   - Import replaces the directories, so extracting directly would mean
//     emptying them first. A truncated or corrupt archive would then lose the
//     existing files, while a failed extraction into staging leaves them as is.
//   - Directories may be symlinks into a shared directory, as in SSH
//     deployments. tar replaces such symlinks with real directories unless GNU
//     tar's --keep-directory-symlink is used, which bsdtar and BusyBox tar lack.
//     Moving the extracted contents keeps the symlinks.
//
// The staging directory lives in the first directory instead of the root, as
// the root may be read-only, like the public directory of a sealed SSH
// release, while the directories themselves link to writable shared ones.
// find runs commands with "+", so a failing rm or mv fails the script.
const unpackScript = `set -e
mkdir -p "$1"
cd "$1"
shift
mkdir -p "$@"
staging=$(mktemp -d "$1/.shopware-import.XXXXXX")
trap 'rm -rf "$staging"' EXIT
tar -xf - -C "$staging"
staging_name=$(basename "$staging")
for dir in "$@"; do
	find "$dir/" -mindepth 1 -maxdepth 1 ! -name "$staging_name" -exec rm -rf {} +
	if [ -d "$staging/$dir" ]; then
		find "$staging/$dir/" -mindepth 1 -maxdepth 1 -exec sh -c 'mv "$@" "$0"' "$dir/" {} +
	fi
done`

// Filesystem is a Shopware filesystem configured in shopware.filesystem.
type Filesystem struct {
	Type   string `json:"type"`
	Config struct {
		Root string `json:"root"`
	} `json:"config"`
}

// Filesystems are the public and private Shopware filesystems of a project.
type Filesystems struct {
	Public  Filesystem `json:"public"`
	Private Filesystem `json:"private"`
}

// ResolveFilesystems reads the resolved shopware.filesystem configuration of
// the project. The --format option of debug:config requires Symfony 6.3,
// shipped since Shopware 6.5.2.0.
func ResolveFilesystems(ctx context.Context, env Environment) (Filesystems, error) {
	output, err := env.ConsoleCommand(ctx, "debug:config", "shopware", "filesystem", "--resolve-env", "--format=json", "--no-ansi").Output()
	if err != nil {
		return Filesystems{}, fmt.Errorf("cannot read the filesystem configuration: %w", commandError(err))
	}

	return parseFilesystems(output)
}

func parseFilesystems(output []byte) (Filesystems, error) {
	var filesystems Filesystems
	if err := unmarshalConsoleJSON(output, &filesystems); err != nil {
		return Filesystems{}, fmt.Errorf("cannot parse the filesystem configuration: %w", err)
	}

	return filesystems, nil
}

// localRoot returns the root of a local filesystem and rejects other adapters.
// A root with an unresolved parameter like %env(...)% is rejected, as files
// would be read from or written to a literal directory of that name.
func localRoot(name string, fs Filesystem) (string, error) {
	if !strings.EqualFold(fs.Type, localAdapter) {
		return "", fmt.Errorf("cannot transfer files of the %s filesystem: adapter %q is not supported, only %q", name, fs.Type, localAdapter)
	}
	if fs.Config.Root == "" || fs.Config.Root == "/" {
		return "", fmt.Errorf("cannot transfer files of the %s filesystem: invalid root %q", name, fs.Config.Root)
	}
	if strings.Contains(fs.Config.Root, "%") {
		return "", fmt.Errorf("cannot transfer files of the %s filesystem: root %q contains an unresolved parameter", name, fs.Config.Root)
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
	var parameters map[string]string
	if err := unmarshalConsoleJSON(output, &parameters); err != nil {
		return "", fmt.Errorf("cannot detect the Shopware version: %w", err)
	}

	version := parameters[shopwareVersionParameter]
	if version == "" {
		return "", errors.New("cannot detect the Shopware version: parameter " + shopwareVersionParameter + " is empty")
	}

	return version, nil
}

// unmarshalConsoleJSON decodes the JSON object printed by a console command.
// Deprecation notices may precede it.
func unmarshalConsoleJSON(output []byte, v any) error {
	if start := bytes.IndexByte(output, '{'); start > 0 {
		output = output[start:]
	}

	return json.Unmarshal(output, v)
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

// emptyFiles empties the directories below root. It unpacks an empty archive,
// so it shares the symlink handling of unpackFiles.
func emptyFiles(ctx context.Context, env Environment, root string, dirs []string) error {
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	if err := tar.NewWriter(gz).Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}

	return unpackFiles(ctx, env, root, dirs, &archive)
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
