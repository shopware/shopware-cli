package project

import (
	"encoding/json"
	"fmt"

	"github.com/invopop/jsonschema"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/shop"
)

var projectConfigSchemaCmd = &cobra.Command{
	Use:   "config-schema",
	Short: "Print the JSON schema of .shopware-project.yaml",
	RunE: func(cmd *cobra.Command, args []string) error {
		r := new(jsonschema.Reflector)
		r.FieldNameTag = "yaml"
		r.RequiredFromJSONSchemaTags = true

		if err := r.AddGoComments("github.com/shopware/shopware-cli", "./internal/shop"); err != nil {
			return fmt.Errorf("cannot generate project schema comments: %w", err)
		}

		schema := r.Reflect(&shop.Config{})

		bytes, err := json.MarshalIndent(schema, "", "  ")
		if err != nil {
			return fmt.Errorf("cannot marshal project schema: %w", err)
		}

		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(bytes))
		return err
	},
}

func init() {
	projectRootCmd.AddCommand(projectConfigSchemaCmd)
}
