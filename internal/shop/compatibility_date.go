package shop

import "fmt"

const (
	// DevMode breaks
	CompatibilityDevMode = "2026-03-01"
)

var (
	ErrDevModeNotSupported = NewCompatibilityError("development mode is not supported for this compatibility date", CompatibilityDevMode)
)

func NewCompatibilityError(message string, date string) error {
	return &CompatibilityError{
		Message: message,
		date:    date,
	}
}

type CompatibilityError struct {
	Message string
	date    string
}

func (e *CompatibilityError) Error() string {
	return fmt.Sprintf("%s, requires compatibility date: %s. see https://developer.shopware.com/docs/products/cli/project-commands/build.html#compatibility-date for more", e.Message, e.date)
}
