package extension

type NpmPackage struct {
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	Scripts         map[string]string `json:"scripts"`
}

func (p NpmPackage) HasScript(name string) bool {
	_, ok := p.Scripts[name]
	return ok
}

// When a package is defined in both dependencies and devDependencies, bun will crash.
func canRunBunOnPackage(npmPackage NpmPackage) bool {
	for name := range npmPackage.Dependencies {
		if _, ok := npmPackage.DevDependencies[name]; ok {
			return false
		}
	}

	return true
}
