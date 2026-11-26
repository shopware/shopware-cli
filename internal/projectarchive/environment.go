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

// fileComponent describes how a Shopware filesystem is stored in an archive.
type fileComponent struct {
	name  string
	entry string
	// dirs are the directories below the filesystem root that belong to an
	// archive.
	dirs []string
	// public distinguishes the public from the private filesystem.
	public bool
}

// The public filesystem, of which only media and thumbnails belong to an
// archive, and the whole private filesystem.
var (
	publicFiles    = fileComponent{name: "public", entry: publicFilesEntry, dirs: []string{"media", "thumbnail"}, public: true}
	privateFiles   = fileComponent{name: "private", entry: privateFilesEntry, dirs: []string{"."}}
	fileComponents = []fileComponent{publicFiles, privateFiles}
)

// selected reports whether components include the file component.
func (c fileComponent) selected(components Components) bool {
	if c.public {
		return components.PublicFiles
	}

	return components.PrivateFiles
}

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
// --no-same-owner gives extracted files to the importing user, as the source
// owners are meaningless on the target; only root would restore them.
const unpackScript = `set -e
mkdir -p "$1"
cd "$1"
shift
mkdir -p "$@"
staging=$(mktemp -d "$1/.shopware-import.XXXXXX")
trap 'rm -rf "$staging"' EXIT
tar -xf - --no-same-owner -C "$staging"
staging_name=$(basename "$staging")
for dir in "$@"; do
	find "$dir/" -mindepth 1 -maxdepth 1 ! -name "$staging_name" -exec rm -rf {} +
	if [ -d "$staging/$dir" ]; then
		find "$staging/$dir/" -mindepth 1 -maxdepth 1 -exec sh -c 'mv "$@" "$0"' "$dir/" {} +
	fi
done`

// filesystemConfig is a Shopware filesystem configured in shopware.filesystem.
type filesystemConfig struct {
	Type   string `json:"type"`
	Config struct {
		Root string `json:"root"`
	} `json:"config"`
}

// filesystemsConfig are the public and private Shopware filesystems of a
// project.
type filesystemsConfig struct {
	Public  filesystemConfig `json:"public"`
	Private filesystemConfig `json:"private"`
}

// FileRoots are the local roots of the Shopware filesystems an export reads
// or an import writes. Only ResolveFileRoots creates them, so every root is
// known to be usable.
type FileRoots struct {
	// public and private are empty when the component is not needed.
	public, private string
}

// root returns the root of the component's filesystem.
func (r FileRoots) root(fc fileComponent) (string, error) {
	root := r.private
	if fc.public {
		root = r.public
	}
	if root == "" {
		return "", fmt.Errorf("cannot transfer %s files: the filesystem was not resolved", fc.name)
	}

	return root, nil
}

// ResolveFileRoots reads the resolved shopware.filesystem configuration of the
// project and returns the roots of the needed file components. It rejects
// filesystems an archive cannot be transferred to or from, so this happens
// before any data is read or written. The --format option of debug:config
// requires Symfony 6.3, shipped since Shopware 6.5.2.0.
func ResolveFileRoots(ctx context.Context, env Environment, needed Components) (FileRoots, error) {
	if !needed.PublicFiles && !needed.PrivateFiles {
		return FileRoots{}, nil
	}

	output, err := env.ConsoleCommand(ctx, "debug:config", "shopware", "filesystem", "--resolve-env", "--format=json", "--no-ansi").Output()
	if err != nil {
		return FileRoots{}, fmt.Errorf("cannot read the filesystem configuration: %w", commandError(err))
	}

	return parseFileRoots(output, needed)
}

func parseFileRoots(output []byte, needed Components) (FileRoots, error) {
	var filesystems filesystemsConfig
	if err := unmarshalConsoleJSON(output, &filesystems); err != nil {
		return FileRoots{}, fmt.Errorf("cannot parse the filesystem configuration: %w", err)
	}

	var roots FileRoots
	var err error
	if needed.PublicFiles {
		if roots.public, err = publicFiles.localRoot(filesystems.Public); err != nil {
			return FileRoots{}, err
		}
	}
	if needed.PrivateFiles {
		if roots.private, err = privateFiles.localRoot(filesystems.Private); err != nil {
			return FileRoots{}, err
		}
	}

	return roots, nil
}

// localRoot returns the root of the component's local filesystem and rejects
// other adapters. A root with an unresolved parameter like %env(...)% is
// rejected, as files would be read from or written to a literal directory of
// that name.
func (c fileComponent) localRoot(fs filesystemConfig) (string, error) {
	name := c.name
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
