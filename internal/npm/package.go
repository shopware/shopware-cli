package npm

import (
	"encoding/json"
	"os"
	"path"
)

// Package represents a parsed package.json file.
type Package struct {
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	Scripts         map[string]string `json:"scripts"`
}

// HasScript checks if the package.json contains a script with the given name.
func (p Package) HasScript(name string) bool {
	_, ok := p.Scripts[name]
	return ok
}

// HasDevDependency checks if the package.json contains a dev dependency with the given name.
func (p Package) HasDevDependency(name string) bool {
	_, ok := p.DevDependencies[name]
	return ok
}

// ReadPackage reads and parses a package.json file from the given directory.
func ReadPackage(dir string) (Package, error) {
	packageJsonFile, err := os.ReadFile(path.Join(dir, "package.json"))
	if err != nil {
		return Package{}, err
	}

	var pkg Package
	if err := json.Unmarshal(packageJsonFile, &pkg); err != nil {
		return Package{}, err
	}
	return pkg, nil
}

// NodeModulesExists checks if a node_modules directory exists in the given root.
func NodeModulesExists(root string) bool {
	if _, err := os.Stat(path.Join(root, "node_modules")); err == nil {
		return true
	}
	return false
}
