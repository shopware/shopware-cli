package extension

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shopware/shopware-cli/internal/validation"
)

func validateTheme(ext Extension, check validation.Check) {
	themeJSONPath := filepath.Join(ext.GetRootDir(), "Resources/theme.json")

	if _, err := os.Stat(themeJSONPath); !os.IsNotExist(err) {
		content, err := os.ReadFile(themeJSONPath)
		if err != nil {
			check.AddResult(validation.CheckResult{
				Path:       "Resources/theme.json",
				Identifier: "theme.validator",
				Message:    "Invalid theme.json",
				Severity:   validation.SeverityError,
			})
			return
		}

		var theme themeJSON
		err = json.Unmarshal(content, &theme)
		if err != nil {
			check.AddResult(validation.CheckResult{
				Path:       "Resources/theme.json",
				Identifier: "theme.validator",
				Message:    "Cannot decode theme.json",
				Severity:   validation.SeverityError,
			})
			return
		}

		if len(theme.PreviewMedia) == 0 {
			check.AddResult(validation.CheckResult{
				Path:       "Resources/theme.json",
				Identifier: "theme.validator",
				Message:    "Required field \"previewMedia\" missing in theme.json",
				Severity:   validation.SeverityError,
			})
			return
		}

		expectedMediaPath := fmt.Sprintf("%s/src/Resources/%s", ext.GetPath(), theme.PreviewMedia)

		if _, err := os.Stat(expectedMediaPath); os.IsNotExist(err) {
			check.AddResult(validation.CheckResult{
				Path:       "Resources/theme.json",
				Identifier: "theme.validator",
				Message:    fmt.Sprintf("Theme preview image file is expected to be placed at %s, but not found there.", expectedMediaPath),
				Severity:   validation.SeverityError,
			})
		}
	}
}

type themeJSON struct {
	PreviewMedia string `json:"previewMedia"`
}
