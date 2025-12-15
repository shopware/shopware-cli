package extension

import (
	"image"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResizeExtensionIcon(t *testing.T) {
	t.Run("no icon path", func(t *testing.T) {
		ext := &mockExtension{iconPath: ""}
		err := ResizeExtensionIcon(getTestContext(), ext)
		assert.NoError(t, err)
	})

	t.Run("icon is smaller than 56x56", func(t *testing.T) {
		tempDir := t.TempDir()
		iconPath := filepath.Join(tempDir, "icon.png")
		assert.NoError(t, createTestImageWithSize(iconPath, 32, 32))

		ext := &mockExtension{iconPath: iconPath}
		err := ResizeExtensionIcon(getTestContext(), ext)
		assert.NoError(t, err)

		f, err := os.Open(iconPath)
		assert.NoError(t, err)

		img, _, err := image.DecodeConfig(f)
		assert.NoError(t, err)
		assert.Equal(t, 32, img.Width)
		assert.Equal(t, 32, img.Height)
		assert.NoError(t, f.Close(), "Failed to close file after resizing icon")
	})

	t.Run("icon is exactly 56x56", func(t *testing.T) {
		tempDir := t.TempDir()
		iconPath := filepath.Join(tempDir, "icon.png")
		assert.NoError(t, createTestImageWithSize(iconPath, 32, 32))

		ext := &mockExtension{iconPath: iconPath}
		err := ResizeExtensionIcon(getTestContext(), ext)
		assert.NoError(t, err)

		f, err := os.Open(iconPath)
		assert.NoError(t, err)

		img, _, err := image.DecodeConfig(f)
		assert.NoError(t, err)
		assert.Equal(t, 32, img.Width)
		assert.Equal(t, 32, img.Height)
		assert.NoError(t, f.Close(), "Failed to close file after resizing icon")
	})

	t.Run("icon is larger than 56x56", func(t *testing.T) {
		tempDir := t.TempDir()
		iconPath := filepath.Join(tempDir, "icon.png")
		assert.NoError(t, createTestImageWithSize(iconPath, 32, 32))

		ext := &mockExtension{iconPath: iconPath}
		err := ResizeExtensionIcon(getTestContext(), ext)
		assert.NoError(t, err)

		f, err := os.Open(iconPath)
		assert.NoError(t, err)

		img, _, err := image.DecodeConfig(f)
		assert.NoError(t, err)
		assert.Equal(t, 32, img.Width)
		assert.Equal(t, 32, img.Height)
		assert.NoError(t, f.Close(), "Failed to close file after resizing icon")
	})
}
