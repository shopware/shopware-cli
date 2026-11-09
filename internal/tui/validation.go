package tui

import (
	"errors"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
)

// CombineValidators runs validators in order and returns the first error.
// Every validator receives the original, unmodified value.
func CombineValidators(validators ...func(string) error) func(string) error {
	return func(value string) error {
		for _, validate := range validators {
			if err := validate(value); err != nil {
				return err
			}
		}
		return nil
	}
}

// OptionalValidator skips validation only for an empty string, not whitespace.
func OptionalValidator(validate func(string) error) func(string) error {
	return func(value string) error {
		if value == "" {
			return nil
		}
		return validate(value)
	}
}

// ValidateRequired rejects empty and whitespace-only values.
func ValidateRequired(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("this cannot be empty")
	}
	return nil
}

// ValidateSingleLine rejects carriage returns, line feeds and NUL bytes.
// Empty values are allowed; combine with ValidateRequired when needed.
func ValidateSingleLine(value string) error {
	if strings.ContainsAny(value, "\r\n\x00") {
		return errors.New("value must be a single line")
	}
	return nil
}

// ValidateHTTPURL requires an absolute HTTP(S) URL without credentials.
func ValidateHTTPURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return errors.New("enter an absolute http or https URL without credentials")
	}
	return nil
}

// ValidateEmail accepts a single bare email address, without a display name.
func ValidateEmail(value string) error {
	if err := ValidateSingleLine(value); err != nil {
		return err
	}
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address != value {
		return errors.New("enter a valid email address")
	}
	return nil
}

// ValidatePort accepts TCP port numbers from 1 through 65535.
func ValidatePort(value string) error {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("enter a valid TCP port")
	}
	return nil
}
