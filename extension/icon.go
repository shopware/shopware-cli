package extension

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"os"

	"golang.org/x/image/draw"

	"github.com/shopware/shopware-cli/logging"
)

func ResizeExtensionIcon(ctx context.Context, ext Extension) error {
	// Resize the icon to 256x256 if it's larger than that.
	iconPath := ext.GetIconPath()
	if iconPath == "" {
		return nil // No icon to resize
	}

	file, err := os.Open(iconPath)
	if err != nil {
		return fmt.Errorf("cannot open icon: %w", err)
	}

	src, _, err := image.Decode(file)
	if err := file.Close(); err != nil {
		return fmt.Errorf("cannot close icon file: %w", err)
	}

	if err != nil {
		return fmt.Errorf("cannot decode icon: %w", err)
	}

	if src.Bounds().Dx() <= 256 && src.Bounds().Dy() <= 256 {
		return nil
	}

	logging.FromContext(ctx).Infof("Resizing extension icon from %dx%d to 256x256", src.Bounds().Dx(), src.Bounds().Dy())

	dst := image.NewRGBA(image.Rect(0, 0, 256, 256))

	draw.BiLinear.Scale(dst, dst.Rect, src, src.Bounds(), draw.Over, nil)

	// Open file for writing
	writeFile, err := os.Create(iconPath)
	if err != nil {
		return fmt.Errorf("cannot create icon file: %w", err)
	}

	encoder := png.Encoder{
		CompressionLevel: png.BestCompression,
	}

	if err := encoder.Encode(writeFile, dst); err != nil {
		return fmt.Errorf("cannot encode icon: %w", err)
	}

	return writeFile.Close()
}
