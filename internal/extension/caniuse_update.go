package extension

import (
	"encoding/json"
	"os"
	"strings"
)

func patchPackageLockToRemoveCanIUsePackage(packageJsonPath string) error {
	body, err := os.ReadFile(packageJsonPath)

	if err != nil {
		return err
	}

	var lock map[string]interface{}

	if err := json.Unmarshal(body, &lock); err != nil {
		return err
	}

	if dependencies, ok := lock["dependencies"]; !ok {
		if mappedDeps, ok := dependencies.(map[string]interface{}); ok {
			delete(mappedDeps, "caniuse-lite")
		}
	}

	removeCanIUsePackage(lock)

	updatedBody, err := json.MarshalIndent(lock, "", "  ")

	if err != nil {
		return err
	}

	return os.WriteFile(packageJsonPath, updatedBody, os.ModePerm)
}

func removeCanIUsePackage(pkg map[string]interface{}) {
	if dependencies, ok := pkg["dependencies"]; ok {
		if mappedDeps, ok := dependencies.(map[string]interface{}); ok {
			delete(mappedDeps, "caniuse-lite")

			for _, dep := range mappedDeps {
				if depMap, ok := dep.(map[string]interface{}); ok {
					removeCanIUsePackage(depMap)
				}
			}
		}
	}

	if packages, ok := pkg["packages"].(map[string]interface{}); ok {
		for name := range packages {
			if strings.HasSuffix(name, "caniuse-lite") {
				delete(packages, name)
			}
		}
	}
}
