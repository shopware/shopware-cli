package extension

import (
	"errors"
	"fmt"
	"regexp"
)

// Shopware technical names use UpperCamelCase. Community Store plugins also
// need a vendor prefix, for example SwagBasicExample.
var (
	extensionNameRegexp      = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*$`)
	storeExtensionNameRegexp = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*[A-Z][A-Za-z0-9]*$`)
)

func ValidateName(name string, store bool) error {
	if name == "" {
		return errors.New("extension name must not be empty")
	}
	if store {
		if !storeExtensionNameRegexp.MatchString(name) {
			return fmt.Errorf("invalid extension name %q: Community Store extensions need UpperCamelCase with a vendor prefix, letters and digits only (for example SwagBasicExample)", name)
		}
		return nil
	}
	if !extensionNameRegexp.MatchString(name) {
		return fmt.Errorf("invalid extension name %q: use UpperCamelCase, letters and digits only (for example Example or SwagBasicExample)", name)
	}

	return nil
}

func ValidateType(extensionType ExtensionType) error {
	switch extensionType {
	case Plugin, Theme:
		return nil
	default:
		return fmt.Errorf("invalid extension type %q", extensionType)
	}
}
