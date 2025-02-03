package spdx

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewSpdxLicenses(t *testing.T) {
	_, err := NewSpdxLicenses()
	assert.NoError(t, err)
}

func TestSpdxLicenses_Validate(t *testing.T) {
	s, _ := NewSpdxLicenses()

	tests := []struct {
		identifier string
	}{
		{"MIT"},
		{"mit"},
		{"LGPL-2.1-only"},
		{"GPL-3.0-or-later"},
		{"(LGPL-2.1-only or GPL-3.0-or-later)"},
	}

	for _, tt := range tests {
		t.Run(tt.identifier, func(t *testing.T) {
			bl, err := s.Validate(tt.identifier)

			assert.NoError(t, err)
			assert.True(t, bl)
		})
	}
}
