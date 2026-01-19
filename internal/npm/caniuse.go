package npm

import (
	"encoding/json"
	"os"
	"strings"
)

// PatchPackageLockToRemoveCanIUse removes caniuse-lite from a package-lock.json file.
// This is needed because the caniuse-lite package is often outdated in lock files
// and causes issues with browserslist.
func PatchPackageLockToRemoveCanIUse(packageLockPath string) error {
	body, err := os.ReadFile(packageLockPath)
	if err != nil {
		return err
	}

	var lock map[string]any
	if err := json.Unmarshal(body, &lock); err != nil {
		return err
	}

	if dependencies, ok := lock["dependencies"]; !ok {
		if mappedDeps, ok := dependencies.(map[string]any); ok {
			delete(mappedDeps, "caniuse-lite")
		}
	}

	removeCanIUsePackage(lock)

	updatedBody, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(packageLockPath, updatedBody, os.ModePerm)
}

func removeCanIUsePackage(pkg map[string]any) {
	if dependencies, ok := pkg["dependencies"]; ok {
		if mappedDeps, ok := dependencies.(map[string]any); ok {
			delete(mappedDeps, "caniuse-lite")

			for _, dep := range mappedDeps {
				if depMap, ok := dep.(map[string]any); ok {
					removeCanIUsePackage(depMap)
				}
			}
		}
	}

	if packages, ok := pkg["packages"].(map[string]any); ok {
		for name := range packages {
			if strings.HasSuffix(name, "caniuse-lite") {
				delete(packages, name)
			}
		}
	}
}
