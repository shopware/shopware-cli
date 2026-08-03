package system

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
)

// PHPVersionNotFoundError reports that the PHP version pinned by the project
// config is not installed on this machine.
type PHPVersionNotFoundError struct {
	Pin           string
	Installations []PHPInstallation
}

func (e *PHPVersionNotFoundError) Error() string {
	var b strings.Builder

	fmt.Fprintf(&b, "this project requires PHP %s (php_version in .shopware-project.yml), but no PHP %s was found on this machine", e.Pin, e.Pin)

	if len(e.Installations) > 0 {
		found := make([]string, 0, len(e.Installations))
		for _, installation := range e.Installations {
			found = append(found, fmt.Sprintf("PHP %s (%s)", installation.Version, installation.Source))
		}
		fmt.Fprintf(&b, "; discovered %s", strings.Join(found, ", "))
	}

	if hint := installPHPHint(e.Pin); hint != "" {
		fmt.Fprintf(&b, "; install it with %s", hint)
	}

	return b.String()
}

// installPHPHint returns a platform-appropriate command for installing the
// pinned PHP version, or an empty string when there is no obvious one.
func installPHPHint(pin string) string {
	switch runtime.GOOS {
	case "darwin":
		return fmt.Sprintf("`brew install php@%s`", pin)
	case "linux":
		return fmt.Sprintf("`apt install php%s` (or the equivalent for your distribution)", pin)
	default:
		return ""
	}
}

// ResolveProjectPHPBinary returns the PHP executable a project's local commands
// run, following the precedence: PHP_BINARY > the pinned php_version > the php
// found in PATH (an empty return value). An unusable PHP_BINARY is an error
// rather than a reason to fall back to another PHP.
func ResolveProjectPHPBinary(ctx context.Context, pin string) (string, error) {
	if phpBinary := os.Getenv("PHP_BINARY"); phpBinary != "" {
		probed, err := ProbePHPBinary(ctx, phpBinary, PHPSourceEnv)
		if err != nil {
			return "", unusablePHPBinaryError(err)
		}

		return probed.Binary, nil
	}

	return ResolvePinnedPHPBinary(ctx, pin)
}

// ResolvePinnedPHPBinary returns the PHP executable matching the version pinned
// by the project config, ignoring PHP_BINARY. An empty pin returns an empty
// string. A pin that matches no installed PHP fails with
// *PHPVersionNotFoundError instead of falling back to another version.
func ResolvePinnedPHPBinary(ctx context.Context, pin string) (string, error) {
	if pin == "" {
		return "", nil
	}

	// Not phpdiscover.Discover: a pin can come from a PHP_BINARY-only install that
	// the library does not scan, and only this list carries that entry.
	installations := DiscoverPHPInstallations(ctx)

	if installation := FindPHPByVersionPin(installations, pin); installation != nil {
		return installation.Binary, nil
	}

	return "", &PHPVersionNotFoundError{Pin: PHPVersionPin(pin), Installations: installations}
}
