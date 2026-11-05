package tui

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCombineValidators checks validation order and propagation of the first failure.
func TestCombineValidators(t *testing.T) {
	expected := errors.New("invalid input")
	var calls []string
	validate := CombineValidators(
		func(value string) error {
			calls = append(calls, "first")
			assert.Equal(t, "  password  ", value)
			return nil
		},
		func(value string) error {
			calls = append(calls, "second")
			assert.Equal(t, "  password  ", value)
			return expected
		},
		func(string) error {
			t.Fatal("validators after a failure must not run")
			return nil
		},
	)
	require.ErrorIs(t, validate("  password  "), expected)
	assert.Equal(t, []string{"first", "second"}, calls)
	require.NoError(t, CombineValidators()("anything"))

	requiredLine := CombineValidators(ValidateRequired, ValidateSingleLine)
	require.NoError(t, requiredLine("  keep spaces  "))
	require.Error(t, requiredLine(" \t "))
	require.Error(t, requiredLine("first\nsecond"))
}

// TestOptionalValidator checks that only an empty string bypasses validation.
func TestOptionalValidator(t *testing.T) {
	expected := errors.New("invalid input")
	var values []string
	validate := OptionalValidator(func(value string) error {
		values = append(values, value)
		return expected
	})
	require.NoError(t, validate(""))
	assert.Empty(t, values)
	require.ErrorIs(t, validate(" "), expected)
	require.ErrorIs(t, validate("  password  "), expected)
	assert.Equal(t, []string{" ", "  password  "}, values)

	email := OptionalValidator(ValidateEmail)
	require.NoError(t, email(""))
	require.NoError(t, email("admin@example.com"))
	require.Error(t, email(" "))
}

// TestInputValidators checks accepted and rejected form input values.
func TestInputValidators(t *testing.T) {
	for _, test := range []struct {
		name     string
		validate func(string) error
		valid    []string
		invalid  []string
	}{
		{
			name: "required", validate: ValidateRequired,
			valid:   []string{"value", " value ", "pässword"},
			invalid: []string{"", " ", "\t\r\n", "\u2003"},
		},
		{
			name: "single line", validate: ValidateSingleLine,
			valid:   []string{"", " value ", "one\ttwo"},
			invalid: []string{"one\ntwo", "one\rtwo", "one\x00two"},
		},
		{
			name: "HTTP URL", validate: ValidateHTTPURL,
			valid:   []string{"https://shop.example.com", "http://localhost:8000/path?key=value", "https://[::1]/"},
			invalid: []string{"", " ", "shop.example.com", "//shop.example.com", "ftp://shop.example.com", "https:///path", "https://user:secret@shop.example.com", "https://shop.example.com\n", "https://shop.example.com\x00"},
		},
		{
			name: "email", validate: ValidateEmail,
			valid:   []string{"admin@example.com", "user+tag@example.com"},
			invalid: []string{"", " ", "admin", "Admin <admin@example.com>", "admin@example.com,other@example.com", " admin@example.com", "admin@example.com\r\n", "admin\x00@example.com"},
		},
		{
			name: "port", validate: ValidatePort,
			valid:   []string{"1", "3306", "65535"},
			invalid: []string{"", "0", "-1", "65536", "3306.0", " 3306", "mysql", "99999999999999999999999999"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, value := range test.valid {
				assert.NoError(t, test.validate(value), "%q", value)
			}
			for _, value := range test.invalid {
				assert.Error(t, test.validate(value), "%q", value)
			}
		})
	}
}
