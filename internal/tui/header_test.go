package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRenderProjectHeader_ContainsProjectAndVersion(t *testing.T) {
	header := RenderProjectHeader(100, "acme-shop", "local", "6.6.10.3")

	assert.Contains(t, header, "Shopware CLI")
	assert.Contains(t, header, "acme-shop")
	assert.Contains(t, header, "local")
	assert.Contains(t, header, "Shopware 6.6.10.3")
}

func TestRenderProjectHeader_IsTwoLines(t *testing.T) {
	header := RenderProjectHeader(100, "acme-shop", "local", "6.6.10.3")

	assert.Len(t, strings.Split(header, "\n"), 2)
}
