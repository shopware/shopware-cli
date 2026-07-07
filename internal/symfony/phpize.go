package symfony

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	integerRegexp = regexp.MustCompile(`^-?[0-9]+$`)
	binaryRegexp  = regexp.MustCompile(`^0b[01]+$`)
	hexRegexp     = regexp.MustCompile(`(?i)^0x[0-9a-f]+$`)
	floatRegexp   = regexp.MustCompile(`^[+-]?([0-9]+(\.[0-9]*)?|\.[0-9]+)([eE][+-]?[0-9]+)?$`)
)

// phpize converts the string content of a XML configuration value into its PHP
// representation, mirroring Symfony's XmlUtils::phpize. It returns nil, bool,
// int64, float64 or string.
func phpize(value string) any {
	lower := strings.ToLower(value)

	switch {
	case value == "":
		return ""
	case lower == "null":
		return nil
	case lower == "true":
		return true
	case lower == "false":
		return false
	case integerRegexp.MatchString(value):
		return phpizeInteger(value)
	case binaryRegexp.MatchString(value):
		if parsed, err := strconv.ParseInt(value[2:], 2, 64); err == nil {
			return parsed
		}

		return value
	case hexRegexp.MatchString(value):
		if parsed, err := strconv.ParseInt(value[2:], 16, 64); err == nil {
			return parsed
		}

		return value
	case floatRegexp.MatchString(value):
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			return parsed
		}

		return value
	default:
		return value
	}
}

// phpizeInteger handles digit-only values like XmlUtils::phpize does: values
// with a single leading zero are interpreted as octal numbers, values that do
// not survive an integer round trip (e.g. "08") stay strings.
func phpizeInteger(value string) any {
	digits := strings.TrimPrefix(value, "-")

	if len(digits) > 1 && digits[0] == '0' {
		if octal, err := strconv.ParseInt(value, 8, 64); err == nil && digits == "0"+strconv.FormatInt(abs(octal), 8) {
			return octal
		}

		return value
	}

	if parsed, err := strconv.ParseInt(value, 10, 64); err == nil && strconv.FormatInt(parsed, 10) == value {
		return parsed
	}

	return value
}

func abs(value int64) int64 {
	if value < 0 {
		return -value
	}

	return value
}
