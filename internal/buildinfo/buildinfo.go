// Package buildinfo holds build-time metadata of the CLI binary.
package buildinfo

// Version is the CLI version, set at build time via
// -ldflags "-X 'github.com/shopware/shopware-cli/internal/buildinfo.Version=1.2.3'".
var Version = "dev"

// UserAgent returns the User-Agent header value for outgoing HTTP requests.
func UserAgent() string {
	return "shopware-cli/" + Version
}
