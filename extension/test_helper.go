package extension

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"

	"github.com/shopware/shopware-cli/internal/validation"
)

func createTestImage(path string) error {
	return createTestImageWithSize(path, 128, 128)
}

func createTestImageWithSize(path string, width, height int) error {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Create a simple pattern with fewer colors for better compression
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			// Simple checkerboard pattern with just 2 colors
			if (x/4+y/4)%2 == 0 {
				img.Set(x, y, color.RGBA{220, 220, 220, 255}) // Light gray
			} else {
				img.Set(x, y, color.RGBA{180, 180, 180, 255}) // Slightly darker gray
			}
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}

	defer func() {
		_ = f.Close()
	}()

	encoder := png.Encoder{
		CompressionLevel: png.BestCompression,
	}

	return encoder.Encode(f, img)
}

// testCheck is a test helper that implements validation.Check
type testCheck struct {
	Results []validation.CheckResult
}

func (c *testCheck) AddResult(result validation.CheckResult) {
	c.Results = append(c.Results, result)
}

func (c *testCheck) GetResults() []validation.CheckResult {
	return c.Results
}

func (c *testCheck) HasErrors() bool {
	for _, r := range c.Results {
		if r.Severity == validation.SeverityError {
			return true
		}
	}
	return false
}

func (c *testCheck) RemoveByIdentifier(ignores []validation.ToolConfigIgnore) validation.Check {
	filtered := make([]validation.CheckResult, 0)
	for _, r := range c.Results {
		shouldKeep := true
		for _, ignore := range ignores {
			// Only ignore all matches when identifier is the only field specified
			if ignore.Identifier != "" && ignore.Path == "" && ignore.Message == "" {
				if r.Identifier == ignore.Identifier {
					shouldKeep = false
					break
				}
			}

			// If path is specified with identifier (but no message), match both
			if ignore.Identifier != "" && ignore.Path != "" && ignore.Message == "" {
				if r.Identifier == ignore.Identifier && r.Path == ignore.Path {
					shouldKeep = false
					break
				}
			}

			// If both identifier and message are specified, match both
			if ignore.Identifier != "" && ignore.Message != "" && r.Identifier == ignore.Identifier && strings.Contains(r.Message, ignore.Message) {
				shouldKeep = false
				break
			}

			// Handle message-based ignores (when no identifier is specified)
			if ignore.Identifier == "" && ignore.Message != "" && strings.Contains(r.Message, ignore.Message) && (r.Path == ignore.Path || ignore.Path == "") {
				shouldKeep = false
				break
			}
		}
		if shouldKeep {
			filtered = append(filtered, r)
		}
	}
	c.Results = filtered
	return c
}
