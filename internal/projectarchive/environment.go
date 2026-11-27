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

	"github.com/shopware/shopware-cli/logging"
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
//
// A running shop keeps adding, writing, and deleting files while they are
// archived. GNU tar reports this with exit code 1 but still writes a valid
// archive, so it counts as success and tar's messages become a warning. Other
// tars do not detect it, or use exit code 1 for real errors and broken archives.
const packScript = `set -e
root=$1
shift
for dir do
	shift
	if [ -e "$root/$dir" ]; then set -- "$@" "$dir"; fi
done
[ $# -gt 0 ] || exec dd if=/dev/zero bs=1024 count=1 2>/dev/null
status=0
COPYFILE_DISABLE=1 tar -chf - -C "$root" "$@" || status=$?
if [ "$status" -eq 1 ] && tar --version 2>/dev/null | grep -q 'GNU tar'; then exit 0; fi
exit "$status"`

// stageScript extracts the tar read from stdin into a new staging directory
// ($2) below the first directory, which is below the root ($1). It does not
// touch the existing files, so a failed extraction or database import leaves
// them as is; swapScript replaces them afterwards. The staging directory lives
// in the first directory instead of the root, as the root may be read-only,
// like the public directory of a sealed SSH release, while the directories
// themselves link to writable shared ones. Living there also inherits a
// setgid group or default ACLs of the directory.
//
// The extracted files take over ownership and permissions of the target, as
// the source's are meaningless there: --no-same-owner gives them to the
// importing user, and as root, they are given to the owner of the target
// directory, e.g. the web server user. Directories take the mode of the target
// directory and files the one of an existing file in it, or of the directory
// without execute bits when there is none. Without root, the files keep the
// importing user; for each directory whose owner can then neither write them
// as owner nor through a group-writable group, it prints the directory, the
// files' owner and the expected one.
//
// Before extracting, it checks that swapScript can remove the existing
// contents, which needs write access to every directory below the target, so
// a failing swap does not leave a replaced database with half-removed files.
const stageScript = `set -e
mkdir -p "$1"
cd "$1"
name=$2
shift 2
mkdir -p "$@"
for dir in "$@"; do
	find "$dir/" -type d -exec sh -c 'for d do
		[ -r "$d" ] && [ -w "$d" ] && [ -x "$d" ] || { echo "no permission to remove the existing contents of $d" >&2; exit 1; }
	done' sh {} +
done
staging="$1/$name"
mkdir "$staging"
tar -xf - --no-same-owner -C "$staging"
mode() { stat -c %a "$1" 2>/dev/null || stat -f %Lp "$1"; }
for dir in "$@"; do
	[ -d "$staging/$dir" ] || continue
	owner=$(ls -nd "$dir/" | awk '{print $3 ":" $4}')
	if [ "$(id -u)" = 0 ]; then
		chown -R "$owner" "$staging/$dir"
	fi
	dir_mode=$(mode "$dir/")
	file=$(find "$dir/" -name "$name" -prune -o -type f -print | head -n 1)
	if [ -n "$file" ]; then
		file_mode=$(mode "$file")
	else
		file_mode=$(printf '%o' $((0$dir_mode & 0666)))
	fi
	find "$staging/$dir" -mindepth 1 -type d -exec chmod "$dir_mode" {} +
	find "$staging/$dir" -type f -exec chmod "$file_mode" {} +
	staged_owner=$(ls -nd "$staging/$dir" | awk '{print $3 ":" $4}')
	if [ "${staged_owner%:*}" != "${owner%:*}" ] &&
		{ [ "${staged_owner#*:}" != "${owner#*:}" ] || [ $((0$dir_mode & 0$file_mode & 020)) -eq 0 ]; }; then
		echo "$dir $staged_owner $owner"
	fi
done`

// swapScript replaces the contents of each directory below the root ($1) with
// the one extracted by stageScript into the staging directory ($2). It moves
// the contents instead of the directories, as they may be symlinks into a
// shared directory, as in SSH deployments. tar would replace such symlinks
// with real directories unless GNU tar's --keep-directory-symlink is used,
// which bsdtar and BusyBox tar lack. The emptied staging directory is left to
// cleanupScript.
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
done`

// cleanupScript removes the staging directory ($2) below the first directory
// ($3) below the root ($1), if it exists. It runs after every import, whether
// the staged files were swapped in or not.
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
	gz := gzip.NewWriter(w)
	messages, err := runScript(ctx, env, packScript, append([]string{root}, dirs...), nil, gz)
	if err != nil {
		return err
	}

	if messages = strings.TrimSpace(messages); messages != "" {
		logger := logging.FromContext(ctx)
		logger.Warnf("Files in %s changed during the export, which is expected while the shop is running. "+
			"This is usually fine: new files are not referenced by the database, as it is exported first. "+
			"Only files deleted or overwritten during the export may be missing or incomplete in the archive. "+
			"Use \"--verbose\" to see the %d messages from tar", root, strings.Count(messages, "\n")+1)
		logger.Debugf("Messages from tar:\n%s", messages)
	}

	return gz.Close()
}

// stagedFiles are the files of a component extracted into a staging
// directory, see stage.
type stagedFiles struct {
	fileComponent
	root string
	// staging is the name of the staging directory below the first of dirs.
	staging string
	// empty reports whether an empty archive was staged, so the swap
	// empties the directories.
	empty bool
	// ownerMismatches are the directories whose files the owner of the
	// target directory may not be able to change.
	ownerMismatches []ownerMismatch
}

// ownerMismatch describes staged files of a directory that belong to another
// user than the target directory, as reported by stageScript.
type ownerMismatch struct {
	dir, owner, expected string
}

// stage extracts the tar.gz read from r into the staging directory below the
// first of the component's directories, without touching the existing files.
func (s *stagedFiles) stage(ctx context.Context, env Environment, r io.Reader) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	// Stop after the first member, so zero padding behind it is not
	// mistaken for another one.
	gz.Multistream(false)

	var output bytes.Buffer
	if err := s.run(ctx, env, stageScript, gz, &output); err != nil {
		return err
	}
	for line := range strings.Lines(output.String()) {
		if fields := strings.Fields(line); len(fields) == 3 {
			s.ownerMismatches = append(s.ownerMismatches, ownerMismatch{dir: fields[0], owner: fields[1], expected: fields[2]})
		}
	}

	// tar may stop reading at its end-of-archive marker, before gzip has
	// verified the checksum at the end of the stream.
	_, err = io.Copy(io.Discard, gz)

	return err
}

// emptyFilesArchive returns an empty tar.gz. Staging it makes the swap empty
// the directories with the same checks and symlink handling as replacing them.
func emptyFilesArchive() (io.Reader, error) {
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	err := errors.Join(tar.NewWriter(gz).Close(), gz.Close())

	return &archive, err
}

// swap replaces the directories with the staged files. The emptied staging
// directory stays until cleanup.
func (s *stagedFiles) swap(ctx context.Context, env Environment) error {
	return s.run(ctx, env, swapScript, nil, nil)
}

// cleanup removes the staging directory, if it exists.
func (s *stagedFiles) cleanup(ctx context.Context, env Environment) error {
	return s.run(ctx, env, cleanupScript, nil, nil)
}

// run runs script with the root, the staging directory name and the
// component's directories as arguments.
func (s *stagedFiles) run(ctx context.Context, env Environment, script string, stdin io.Reader, stdout io.Writer) error {
	_, err := runScript(ctx, env, script, append([]string{s.root, s.staging}, s.dirs...), stdin, stdout)

	return err
}

// runScript runs a shell script with args in the environment and returns what
// it wrote to stderr, which a failure carries in its error instead.
func runScript(ctx context.Context, env Environment, script string, args []string, stdin io.Reader, stdout io.Writer) (string, error) {
	process := env.Command(ctx, "sh", append([]string{"-c", script, "sh"}, args...)...)
	var stderr bytes.Buffer
	process.Cmd.Stdin = stdin
	process.Cmd.Stdout = stdout
	process.Cmd.Stderr = &stderr

	if err := process.Run(); err != nil {
		return "", withStderr(err, stderr.String())
	}

	return stderr.String(), nil
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
