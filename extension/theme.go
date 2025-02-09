package extension

import (
	"encoding/json"
	"fmt"
	"os"
)

func validateTheme(ctx *ValidationContext) {
	themeJSONPath := fmt.Sprintf("%s/src/Resources/theme.json", ctx.Extension.GetPath())

	if _, err := os.Stat(themeJSONPath); !os.IsNotExist(err) {
		content, err := os.ReadFile(themeJSONPath)
		if err != nil {
			ctx.AddError("theme.validator", "Invalid theme.json")
			return
		}

		var theme themeJSON
		err = json.Unmarshal(content, &theme)
		if err != nil {
			ctx.AddError("theme.validator", "Cannot decode theme.json")
			return
		}

		if len(theme.PreviewMedia) == 0 {
			ctx.AddError("theme.validator", "Required field \"previewMedia\" missing in theme.json")
			return
		}

		expectedMediaPath := fmt.Sprintf("%s/src/Resources/%s", ctx.Extension.GetPath(), theme.PreviewMedia)

		if _, err := os.Stat(expectedMediaPath); os.IsNotExist(err) {
			ctx.AddError("theme.validator", fmt.Sprintf("Theme preview image file is expected to be placed at %s, but not found there.", expectedMediaPath))
		}
	}
}

type themeJSON struct {
	PreviewMedia string `json:"previewMedia"`
}
