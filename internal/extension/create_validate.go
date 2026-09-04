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

func ValidateType(extensionType ExtensionType) error {
	switch extensionType {
	case Plugin, Theme:
		return nil
	default:
		return fmt.Errorf("invalid extension type %q", extensionType)
	}
}

func validateCreateOptions(opts CreateOptions) error {
	if err := ValidateType(opts.Type); err != nil {
		return err
	}

	return ValidateName(opts.Name)
}
