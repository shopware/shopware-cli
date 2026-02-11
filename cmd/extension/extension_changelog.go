package extension

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/extension"
)

var extensionChangelogCmd = &cobra.Command{
	Use:   "get-changelog [path]",
	Short: "Get the changelog",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("cannot find path: %w", err)
		}

		stat, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("cannot find path: %w", err)
		}

		var ext extension.Extension

		if stat.IsDir() {
			ext, err = extension.GetExtensionByFolder(cmd.Context(), path)
		} else {
			ext, err = extension.GetExtensionByZip(path)
		}

		if err != nil {
			return fmt.Errorf("changelog: cannot open extension %w", err)
		}

		changelog, err := ext.GetChangelog()
		if err != nil {
			return fmt.Errorf("cannot generate changelog: %w", err)
		}

		requestedLanguage, _ := cmd.Flags().GetString("language")

		if requestedLanguage == "" {
			fmt.Println(changelog.English)
			return nil
		}

		langKeys := strings.Split(requestedLanguage, ",")

		for _, langKey := range langKeys {
			lang, ok := changelog.Changelogs[langKey]

			if ok {
				fmt.Println(lang)
				return nil
			}
		}

		return fmt.Errorf("changelog for language %s not found", requestedLanguage)
	},
}

func init() {
	extensionRootCmd.AddCommand(extensionChangelogCmd)
	extensionChangelogCmd.PersistentFlags().String("language", "", "Language of the changelog, can be multiple specified as fallback (comma separated)")
}
