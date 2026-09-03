package extension

import (
	"errors"
	"fmt"
	"regexp"
)

// Shopware technical names use UpperCamelCase and contain a vendor prefix,
// for example SwagBasicExample.
var extensionNameRegexp = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*[A-Z][A-Za-z0-9]*$`)

func ValidateName(name string) error {
	if name == "" {
		return errors.New("extension name must not be empty")
	}
	if !extensionNameRegexp.MatchString(name) {
		return fmt.Errorf("invalid extension name %q: use UpperCamelCase with a vendor prefix, letters and digits only (for example SwagBasicExample)", name)
	}

	return nil
}
