package system

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	phpdiscover "github.com/shyim/go-php-discover"
	"github.com/shyim/go-version"
)

// Sources a PHPInstallation can be discovered from.
const (
	PHPSourceEnv  = "PHP_BINARY"
	PHPSourcePath = phpdiscover.SourcePath
	PHPSourceFlag = "flag"
)

// PHPInstallation describes a usable PHP executable found on the machine.
type PHPInstallation struct {
	Binary  string // absolute/canonical executable path
	Version string // normalized version reported by the executable
	Source  string // e.g. PHP_BINARY, PATH, homebrew, system package
	Default bool   // true for the executable a plain `php` resolves to on PATH
}

// String renders the installation for user-facing output,
// e.g. "PHP 8.3.19 — /opt/homebrew/opt/php@8.3/bin/php".
func (i PHPInstallation) String() string {
	return "PHP " + i.Version + " — " + i.Binary
}

// DiscoverPHPInstallations returns the usable PHP executables installed on the
// machine via github.com/shyim/go-php-discover, plus an explicitly configured
// PHP_BINARY when set, newest version first with PHP_BINARY hoisted to the front.
func DiscoverPHPInstallations(ctx context.Context) []PHPInstallation {
	found := phpdiscover.Discover(ctx)

	installations := make([]PHPInstallation, 0, len(found)+1)

	// Discover sorts ascending by version; iterate in reverse for newest first.
	for i := len(found) - 1; i >= 0; i-- {
		installations = append(installations, newPHPInstallation(found[i]))
	}

	// The library does not know about PHP_BINARY, so it is probed separately and
	// gets its own source; an already discovered binary is relabelled in place.
	if phpBinary := os.Getenv("PHP_BINARY"); phpBinary != "" {
		if probed, err := ProbePHPBinary(ctx, phpBinary, PHPSourceEnv); err == nil {
			installation := *probed
			if existing := FindPHPByBinary(installations, installation.Binary); existing != nil {
				installation.Default = existing.Default
				installations = slices.DeleteFunc(installations, func(i PHPInstallation) bool {
					return i.Binary == installation.Binary
				})
			}
			installations = append([]PHPInstallation{installation}, installations...)
		}
	}

	return installations
}

// UnusablePHPBinaryEnv reports why a configured PHP_BINARY cannot be used, or nil
// when it is unset or usable. DiscoverPHPInstallations omits an unusable
// PHP_BINARY, so callers reading discovery directly must check this to avoid
// silently falling back to another PHP.
func UnusablePHPBinaryEnv(ctx context.Context) error {
	phpBinary := os.Getenv("PHP_BINARY")
	if phpBinary == "" {
		return nil
	}

	if _, err := ProbePHPBinary(ctx, phpBinary, PHPSourceEnv); err != nil {
		return unusablePHPBinaryError(err)
	}

	return nil
}

func unusablePHPBinaryError(err error) error {
	return fmt.Errorf("PHP_BINARY is set but unusable: %w", err)
}

// newPHPInstallation converts a discovered PHP into a PHPInstallation, dropping
// the distro/RC suffix from its version. The suffix must not survive: go-version
// cannot parse "8.1.2-1ubuntu2.14" at all, so keeping it would exclude Debian
// PHP from every constraint check.
func newPHPInstallation(p *phpdiscover.PHP) PHPInstallation {
	return PHPInstallation{
		Binary:  p.Path,
		Version: fmt.Sprintf("%d.%d.%d", p.Version.Major, p.Version.Minor, p.Version.Patch),
		Source:  p.Source,
		Default: p.IsSystem,
	}
}

// FilterCompatiblePHP returns the installations whose version satisfies the
// given constraint. A nil checker matches everything.
func FilterCompatiblePHP(installations []PHPInstallation, checker PHPVersionChecker) []PHPInstallation {
	out := make([]PHPInstallation, 0, len(installations))
	for _, installation := range installations {
		if checker == nil || checker.Check(installation.Version) {
			out = append(out, installation)
		}
	}
	return out
}

// FindPHPBySource returns the first installation discovered from the given
// source, or nil when there is none.
func FindPHPBySource(installations []PHPInstallation, source string) *PHPInstallation {
	for i := range installations {
		if installations[i].Source == source {
			return &installations[i]
		}
	}
	return nil
}

// PHPVersionPin renders the major.minor pin persisted in the project config
// (e.g. "8.3.19" becomes "8.3"). The patch level is dropped so the pin survives
// a PHP package update.
func PHPVersionPin(phpVersion string) string {
	v, err := version.NewVersion(phpVersion)
	if err != nil {
		return phpVersion
	}

	segments := v.Segments()

	return fmt.Sprintf("%d.%d", segments[0], segments[1])
}

// FindPHPByVersionPin returns the newest installation matching the pin, or nil
// when none does. The pin is a version prefix at any depth: "8" matches every
// 8.x, "8.3" every 8.3.x, "8.3.19" only that patch release. Unlike
// phpdiscover.FindVersion it works on an already discovered list, which also
// carries the PHP_BINARY entry.
func FindPHPByVersionPin(installations []PHPInstallation, pin string) *PHPInstallation {
	if pin == "" {
		return nil
	}

	var best *PHPInstallation

	for i := range installations {
		if !phpVersionHasPrefix(installations[i].Version, pin) {
			continue
		}

		if best == nil || comparePHPVersions(installations[i].Version, best.Version) > 0 {
			best = &installations[i]
		}
	}

	return best
}

// phpVersionHasPrefix reports whether phpVersion falls under the given version
// prefix, comparing whole numeric components so "8.3" does not match "8.30.1".
func phpVersionHasPrefix(phpVersion, prefix string) bool {
	want := strings.Split(prefix, ".")
	got := strings.Split(phpVersion, ".")

	if len(want) > len(got) {
		return false
	}

	for i := range want {
		if want[i] != got[i] {
			return false
		}
	}

	return true
}

// comparePHPVersions orders two PHP version strings, treating unparseable
// versions as lowest.
func comparePHPVersions(a, b string) int {
	va, errA := version.NewVersion(a)
	vb, errB := version.NewVersion(b)

	switch {
	case errA != nil && errB != nil:
		return 0
	case errA != nil:
		return -1
	case errB != nil:
		return 1
	}

	return va.Compare(vb)
}

// FindPHPByBinary returns the installation with the given binary path, or nil
// when the list contains no such entry.
func FindPHPByBinary(installations []PHPInstallation, binary string) *PHPInstallation {
	if binary == "" {
		return nil
	}
	for i := range installations {
		if installations[i].Binary == binary {
			return &installations[i]
		}
	}
	return nil
}

// DefaultPHPInstallation returns the installation a plain `php` resolves to on
// PATH, or nil when there is none.
func DefaultPHPInstallation(installations []PHPInstallation) *PHPInstallation {
	for i := range installations {
		if installations[i].Default {
			return &installations[i]
		}
	}
	return nil
}

// PreferredPHPInstallation returns the installation to preselect for the user:
// the PHP_BINARY candidate when present, otherwise the PATH default, otherwise
// the first entry (the newest version). Returns nil for an empty list.
func PreferredPHPInstallation(installations []PHPInstallation) *PHPInstallation {
	if installation := FindPHPBySource(installations, PHPSourceEnv); installation != nil {
		return installation
	}
	if installation := DefaultPHPInstallation(installations); installation != nil {
		return installation
	}
	if len(installations) > 0 {
		return &installations[0]
	}
	return nil
}

// ProbePHPBinary canonicalizes the given path, verifies that it is an
// executable file, and executes it to obtain its PHP version. It is used to
// validate an explicitly configured PHP binary (e.g. PHP_BINARY).
func ProbePHPBinary(ctx context.Context, path, source string) (*PHPInstallation, error) {
	canonical, ok := canonicalExecutable(path)
	if !ok {
		return nil, &PHPBinaryError{Path: path, Reason: "is not an executable file"}
	}

	phpVersion, err := GetPHPVersionOfBinary(ctx, canonical)
	if err != nil {
		return nil, &PHPBinaryError{Path: path, Reason: "did not report a PHP version", Err: err}
	}

	return &PHPInstallation{Binary: canonical, Version: phpVersion, Source: source}, nil
}

// canonicalExecutable resolves symlinks and relative segments in path and
// reports whether it points to an executable regular file. A bare command name
// (e.g. PHP_BINARY=php8.2) is looked up on PATH first.
func canonicalExecutable(path string) (string, bool) {
	if !strings.ContainsRune(path, filepath.Separator) {
		looked, err := exec.LookPath(path)
		if err != nil {
			return "", false
		}
		path = looked
	}

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", false
	}

	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", false
	}

	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}

	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return "", false
	}

	return resolved, true
}

// PHPBinaryError describes why a specific PHP binary path is not usable.
type PHPBinaryError struct {
	Path   string
	Reason string
	Err    error
}

func (e *PHPBinaryError) Error() string {
	msg := "PHP binary " + e.Path + " " + e.Reason
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

func (e *PHPBinaryError) Unwrap() error {
	return e.Err
}
