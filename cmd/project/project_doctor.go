package project

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
	liplogtable "github.com/charmbracelet/lipgloss/table"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/color"
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

var (
	doctorSectionStyle = lipgloss.NewStyle().Bold(true).Underline(true)
	doctorCheckOK      = color.GreenText.Render("✓")
	doctorCheckWarn    = color.SecondaryText.Render("⚠")
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

		fmt.Println(doctorSectionStyle.Render("Project"))
		fmt.Println()

		shopCfg, err := shop.ReadConfig(cmd.Context(), projectConfigPath, true)
		if err != nil {
			return err
		}

		if shopCfg.IsFallback() {
			fmt.Printf("%s Project config: %s\n", doctorCheckWarn, color.SecondaryText.Render("not found, using fallback"))
		} else {
			fmt.Printf("%s Project config: %s\n", doctorCheckOK, color.GreenText.Render(shop.DefaultConfigFileName()))
		}

		shopwareConstraint, err := extension.GetShopwareProjectConstraint(projectDir)
		if err != nil {
			return err
		}

		fmt.Printf("%s Shopware version: %s\n", doctorCheckOK, color.GreenText.Render(shopwareConstraint.String()))

		fmt.Println()
		fmt.Println(doctorSectionStyle.Render("Detected Extensions & Bundles"))
		fmt.Println()

		sources := extension.FindAssetSourcesOfProject(logging.DisableLogger(cmd.Context()), projectDir, shopCfg)

		if len(sources) == 0 {
			fmt.Printf("%s No extensions or bundles detected\n", doctorCheckWarn)
			return nil
		}

		t := liplogtable.New().
			Border(lipgloss.NormalBorder()).
			Headers("Name", "Path")

		for _, source := range sources {
			relPath, err := filepath.Rel(projectDir, source.Path)
			if err != nil {
				relPath = source.Path
			}
			t.Row(source.Name, relPath)
		}

		fmt.Println(t.Render())

		return nil
	},
}

func init() {
	projectRootCmd.AddCommand(projectDoctor)
}
