// Package cliversion holds the version of the CLI binary.
package cliversion

import (
	"runtime/debug"
	"strings"

	"golang.org/x/mod/module"
)

const devVersion = "dev"

// Version is the CLI version, set at build time via
// -ldflags "-X 'github.com/shopware/shopware-cli/internal/cliversion.Version=1.2.3'".
// Without ldflags it falls back to the module version, e.g. for
// `go install github.com/shopware/shopware-cli@v1.2.3`.
var Version = devVersion

func init() {
	info, _ := debug.ReadBuildInfo()
	Version = resolve(Version, info)
}

// resolve keeps a version set via ldflags and otherwise uses the tagged module
// version from the build info. Untagged builds (pseudo-versions or "(devel)")
// stay "dev".
func resolve(current string, info *debug.BuildInfo) string {
	if current != devVersion || info == nil {
		return current
	}

	moduleVersion := strings.TrimSuffix(info.Main.Version, "+dirty")
	if moduleVersion == "" || moduleVersion == "(devel)" || module.IsPseudoVersion(moduleVersion) {
		return current
	}

	return strings.TrimPrefix(moduleVersion, "v")
}

// UserAgent returns the User-Agent header value for outgoing HTTP requests.
func UserAgent() string {
	return "shopware-cli/" + Version
}
