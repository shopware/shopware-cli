package main

import (
	"encoding/json"
	"os"

	"github.com/invopop/jsonschema"
	"github.com/shopware/shopware-cli/shop"
)

func generateProjectSchema() error {
	r := new(jsonschema.Reflector)
	r.FieldNameTag = "yaml"
	r.RequiredFromJSONSchemaTags = true

	if err := r.AddGoComments("github.com/shopware/shopware-cli", "./shop"); err != nil {
		return err
	}

	schema := r.Reflect(&shop.Config{})

	bytes, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile("shop/shopware-project-schema.json", bytes, 0o644); err != nil {
		return err
	}

	return nil
}

func main() {
	if err := generateProjectSchema(); err != nil {
		panic(err)
	}
}
