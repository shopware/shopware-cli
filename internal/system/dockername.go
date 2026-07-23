package system

import (
	"fmt"
	"regexp"
)

// ComposeProjectNameRegexp matches names that are valid as a Docker Compose
// project name. Docker Compose only allows lowercase letters, digits, dashes
// and underscores, and the name must start with a lowercase letter or digit.
var ComposeProjectNameRegexp = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// ValidateDockerComposeName reports whether name can be used as a Docker
// Compose project name (and, by the same charset rule, as a DNS label).
func ValidateDockerComposeName(name string) error {
	if !ComposeProjectNameRegexp.MatchString(name) {
		return fmt.Errorf("invalid name %q: only lowercase letters, digits, dashes (-) and underscores (_) are allowed, and it must start with a lowercase letter or digit", name)
	}

	return nil
}
