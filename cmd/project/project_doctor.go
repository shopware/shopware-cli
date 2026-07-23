package project

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/logging"
)

var projectDoctor = &cobra.Command{
	Use:   "doctor",
	Short: "Check your Shopware project for potential problems",
	RunE: func(cmd *cobra.Command, args []string) error {
		var err error
		var projectDir string

		if len(args) == 0 {
			projectDir, err = os.Getwd()
			if err != nil {
				return err
			}
		} else {
			projectDir, err = filepath.Abs(args[0])
			if err != nil {
				return err
			}
		}

		fmt.Println(tui.SectionHeadingStyle.Render("Project"))
		fmt.Println()

		shopCfg, err := shop.ReadConfig(cmd.Context(), projectConfigPath, true)
		if err != nil {
			return err
		}

		if shopCfg.IsFallback() {
			fmt.Printf("%s Project config: %s\n", tui.CheckWarn, tui.SecondaryText.Render("not found, using fallback"))
		} else {
			fmt.Printf("%s Project config: %s\n", tui.CheckOK, tui.GreenText.Render(shop.DefaultConfigFileName()))
		}

		shopwareConstraint, err := extension.GetShopwareProjectConstraint(projectDir)
		if err != nil {
			return err
		}

		fmt.Printf("%s Shopware version: %s\n", tui.CheckOK, tui.GreenText.Render(shopwareConstraint.String()))

		fmt.Println()
		fmt.Println(tui.SectionHeadingStyle.Render("Detected Extensions & Bundles"))
		fmt.Println()

		sources := extension.FindAssetSourcesOfProject(logging.DisableLogger(cmd.Context()), projectDir, shopCfg)

		if len(sources) == 0 {
			fmt.Printf("%s No extensions or bundles detected\n", tui.CheckWarn)
			return nil
		}

		rows := make([][]string, 0, len(sources))
		for _, source := range sources {
			relPath, err := filepath.Rel(projectDir, source.Path)
			if err != nil {
				relPath = source.Path
			}
			rows = append(rows, []string{source.Name, relPath})
		}
		tui.PrintTable([]string{"Name", "Path"}, rows)

		return nil
	},
}

func init() {
	projectRootCmd.AddCommand(projectDoctor)
}
