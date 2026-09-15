package extension

import (
	"errors"
	"fmt"
	"regexp"
)

var (
	extensionNameRegexp = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*$`)
	vendorNameRegexp    = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*$`)
)

func ValidateName(name string) error {
	if name == "" {
		return errors.New("extension name must not be empty")
	}

	if !extensionNameRegexp.MatchString(name) {
		return fmt.Errorf("invalid extension name %q: use PascalCase, letters and digits only", name)
	}

	return nil
}

func ValidateVendor(vendor string) error {
	if vendor == "" {
		return errors.New("vendor name must not be empty")
	}

	if !vendorNameRegexp.MatchString(vendor) {
		return fmt.Errorf("invalid vendor name %q: use PascalCase, letters and digits only", vendor)
	}

	return nil
}

func ValidateType(extensionType ExtensionType) error {
	switch extensionType {
	case Plugin, Theme:
		return nil
	default:
		return fmt.Errorf("invalid extension type %q, must be theme or plugin", extensionType)
	}
}
