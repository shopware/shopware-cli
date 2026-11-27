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
// release. Missing directories are skipped instead of created, as the source
// may be read-only, so an unused filesystem exports as an empty tar: two zero
// blocks, as BusyBox tar refuses to create an empty archive. Compression
// happens in Go: tar implementations disagree on padding compressed streams,
// which GNU tar rejects. COPYFILE_DISABLE stops macOS tar from adding
// AppleDouble "._" files for extended attributes, which other tars extract as
// regular files.
const packScript = `set -e
root=$1
shift
for dir do
	shift
	if [ -e "$root/$dir" ]; then set -- "$@" "$dir"; fi
done
[ $# -gt 0 ] || exec dd if=/dev/zero bs=1024 count=1 2>/dev/null
COPYFILE_DISABLE=1 exec tar -chf - -C "$root" "$@"`

// stageScript extracts the tar read from stdin into a new staging directory
// ($2) below the first directory, which is below the root ($1). It does not
// touch the existing files, so a failed extraction or database import leaves
// them as is; swapScript replaces them afterwards. The staging directory lives
// in the first directory instead of the root, as the root may be read-only,
// like the public directory of a sealed SSH release, while the directories
// themselves link to writable shared ones.
// --no-same-owner gives extracted files to the importing user, as the source
// owners are meaningless on the target; only root would restore them.
const stageScript = `set -e
mkdir -p "$1"
cd "$1"
name=$2
shift 2
mkdir -p "$@"
mkdir "$1/$name"
tar -xf - --no-same-owner -C "$1/$name"`

// swapScript replaces the contents of each directory below the root ($1) with
// the one extracted by stageScript into the staging directory ($2). It moves
// the contents instead of the directories, as they may be symlinks into a
// shared directory, as in SSH deployments. tar would replace such symlinks
// with real directories unless GNU tar's --keep-directory-symlink is used,
// which bsdtar and BusyBox tar lack.
// find runs commands with "+", so a failing rm or mv fails the script.
const swapScript = `set -e
cd "$1"
name=$2
shift 2
staging="$1/$name"
for dir in "$@"; do
	find "$dir/" -mindepth 1 -maxdepth 1 ! -name "$name" -exec rm -rf {} +
	if [ -d "$staging/$dir" ]; then
		find "$staging/$dir/" -mindepth 1 -maxdepth 1 -exec sh -c 'mv "$@" "$0"' "$dir/" {} +
	fi
done
rm -rf "$staging"`

// cleanupScript removes the staging directory ($2) below the first directory
// ($3) below the root ($1), if it exists.
const cleanupScript = `cd "$1" 2>/dev/null || exit 0
rm -rf "$3/$2"`

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
// Notices may surround it and contain braces themselves, like "{closure}", so
// the object starts with the first line that starts with a brace.
func unmarshalConsoleJSON(output []byte, v any) error {
	if !bytes.HasPrefix(output, []byte("{")) {
		if start := bytes.Index(output, []byte("\n{")); start >= 0 {
			output = output[start+1:]
		}
	}

	return json.NewDecoder(bytes.NewReader(output)).Decode(v)
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

// stagedFiles are the files of a component extracted into a staging
// directory, see stageFiles.
type stagedFiles struct {
	fileComponent
	root string
	// staging is the name of the staging directory below the first of dirs.
	staging string
	// empty reports whether an empty archive was staged, so the swap
	// empties the directories.
	empty bool
}

// stageFiles extracts the tar.gz read from r into the staging directory below
// the first of the component's directories, without touching the existing
// files. It returns the staged files also on error, so they can be cleaned up.
func stageFiles(ctx context.Context, env Environment, staged stagedFiles, r io.Reader) (stagedFiles, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return staged, err
	}
	// Stop after the first member, so zero padding behind it is not
	// mistaken for another one.
	gz.Multistream(false)

	if err := staged.run(ctx, env, stageScript, gz); err != nil {
		return staged, err
	}

	// tar may stop reading at its end-of-archive marker, before gzip has
	// verified the checksum at the end of the stream.
	_, err = io.Copy(io.Discard, gz)

	return staged, err
}

// stageEmptyFiles stages an empty archive, so swapping empties the
// directories with the same symlink handling.
func stageEmptyFiles(ctx context.Context, env Environment, staged stagedFiles) (stagedFiles, error) {
	staged.empty = true

	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	if err := tar.NewWriter(gz).Close(); err != nil {
		return staged, err
	}
	if err := gz.Close(); err != nil {
		return staged, err
	}

	return stageFiles(ctx, env, staged, &archive)
}

// swap replaces the directories with the staged files and removes the staging
// directory.
func (s stagedFiles) swap(ctx context.Context, env Environment) error {
	return s.run(ctx, env, swapScript, nil)
}

// cleanup removes the staging directory, if it still exists.
func (s stagedFiles) cleanup(ctx context.Context, env Environment) error {
	return s.run(ctx, env, cleanupScript, nil)
}

// run runs script with the root, the staging directory name and the
// component's directories as arguments.
func (s stagedFiles) run(ctx context.Context, env Environment, script string, stdin io.Reader) error {
	process := env.Command(ctx, "sh", append([]string{"-c", script, "sh", s.root, s.staging}, s.dirs...)...)
	var stderr bytes.Buffer
	process.Cmd.Stdin = stdin
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
