package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"

	"github.com/invopop/jsonschema"

	"github.com/shopware/shopware-cli/internal/changelog"
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/shop"
)

var genericTypeRegex = regexp.MustCompile(`^(.+?)\[(.+)]$`)

func getSimpleTypeName(name string) string {
	parts := strings.Split(name, ".")
	return parts[len(parts)-1]
}

func getGenericName(name string) string {
	if matches := genericTypeRegex.FindStringSubmatch(name); matches != nil {
		parent := getSimpleTypeName(matches[1])
		child := getGenericName(matches[2])
		return fmt.Sprintf("%s[%s]", parent, child)
	}
	return getSimpleTypeName(name)
}

func nameGenerics(r reflect.Type) string {
	return getGenericName(r.Name())
}

func generateProjectSchema() error {
	r := new(jsonschema.Reflector)
	r.FieldNameTag = "yaml"
	r.RequiredFromJSONSchemaTags = true

	if err := r.AddGoComments("github.com/shopware/shopware-cli", "./internal/shop"); err != nil {
		return err
	}

	schema := r.Reflect(&shop.Config{})

	bytes, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile("shopware-project-schema.json", bytes, 0o644); err != nil {
		return err
	}

	return nil
}

func generateExtensionSchema() error {
	r := jsonschema.Reflector{Namer: nameGenerics}
	r.FieldNameTag = "yaml"
	r.RequiredFromJSONSchemaTags = true

	if err := r.AddGoComments("github.com/shopware/shopware-cli", "./internal/extension"); err != nil {
		return err
	}

	if err := r.AddGoComments("github.com/shopware/shopware-cli", "./internal/changelog"); err != nil {
		return err
	}

	// Generate the main schema
	schema := r.Reflect(&extension.Config{})

	// Fix the changelog reference in the schema - it's incorrectly mapping to Config
	if definitions, ok := schema.Definitions["Config"]; ok {
		// Find and fix the changelog property
		pair := definitions.Properties.Oldest()
		for pair != nil {
			if pair.Key == "changelog" {
				pair.Value.Ref = "#/$defs/ChangelogConfig"
				break
			}
			pair = pair.Next()
		}
	}

	// Generate the changelog config schema separately and add it to the definitions
	changelogSchema := r.Reflect(&changelog.Config{})
	if schema.Definitions == nil {
		schema.Definitions = make(map[string]*jsonschema.Schema)
	}
	schema.Definitions["ChangelogConfig"] = changelogSchema.Definitions["Config"]

	bytes, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile("shopware-extension-schema.json", bytes, 0o644); err != nil {
		return err
	}

	return nil
}

func main() {
	if err := generateProjectSchema(); err != nil {
		panic(err)
	}

	if err := generateExtensionSchema(); err != nil {
		panic(err)
	}
}
