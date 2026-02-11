package compatibility

import (
	"fmt"
	"time"
)

const dateLayout = "2006-01-02"
const defaultCompatibilityDate = "2026-02-11"

var now = time.Now

// ValidateDate validates a compatibility date in YYYY-MM-DD format.
func ValidateDate(value string) error {
	if value == "" {
		return nil
	}

	if _, err := parseDate(value); err != nil {
		return fmt.Errorf("invalid compatibility_date %q: expected format YYYY-MM-DD", value)
	}

	return nil
}

// IsAtLeast checks whether compatibilityDate is equal to or after requiredDate.
// An empty compatibilityDate falls back to the default compatibility date.
func IsAtLeast(compatibilityDate, requiredDate string) (bool, error) {
	if compatibilityDate == "" {
		compatibilityDate = DefaultDate()
	}

	currentDate, err := parseDate(compatibilityDate)
	if err != nil {
		return false, fmt.Errorf("invalid compatibility_date %q: expected format YYYY-MM-DD", compatibilityDate)
	}

	minDate, err := parseDate(requiredDate)
	if err != nil {
		return false, fmt.Errorf("invalid required compatibility date %q: expected format YYYY-MM-DD", requiredDate)
	}

	return !currentDate.Before(minDate), nil
}

func parseDate(value string) (time.Time, error) {
	return time.Parse(dateLayout, value)
}

func DefaultDate() string {
	return defaultCompatibilityDate
}

func TodayDate() string {
	return now().Format(dateLayout)
}
