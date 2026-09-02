package docker

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// composeFile mirrors only the subset of the compose spec the generator emits.
// Struct field order keeps the output stable; yamlMap keeps map-like sections
// in insertion order.
type composeFile struct {
	Services yamlMap[composeService]         `yaml:"services"`
	Volumes  yamlMap[struct{}]               `yaml:"volumes,omitempty"`
	Networks yamlMap[composeExternalNetwork] `yaml:"networks,omitempty"`
}

// EnvLocalContent is the initial .env.local of a Docker project. It is
// written when the file is missing so compose can start, and by the project
// scaffold.
const EnvLocalContent = "APP_ENV=dev\n"

// WriteCompose renders compose.yaml into the project folder and creates
// .env.local when it is missing.
func (e *Environment) WriteCompose() error {
	composeBytes, err := e.composeYAML()
	if err != nil {
		return err
	}

	if err := ensureEnvLocalFile(e.root); err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(e.root, "compose.yaml"), composeBytes, 0o644)
}

// composeYAML renders the compose file with the managed-file header.
func (e *Environment) composeYAML() ([]byte, error) {
	doc := buildCompose(e)

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, err
	}

	header := "# This file is managed by shopware-cli. Do not edit manually.\n" +
		"# Create a compose.override.yaml to customize services.\n" +
		"# See https://docs.docker.com/compose/how-tos/multiple-compose-files/merge/\n\n"

	return append([]byte(header), out...), nil
}

// ensureEnvLocalFile creates .env.local when it is missing. The generated
// compose file declares it as env_file for every PHP service, and Compose
// refuses to start when a declared env file does not exist — which is the
// state of every fresh clone, since .env.local is gitignored.
func ensureEnvLocalFile(projectFolder string) error {
	envLocalPath := filepath.Join(projectFolder, ".env.local")
	if _, err := os.Stat(envLocalPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return os.WriteFile(envLocalPath, []byte(EnvLocalContent), 0o644)
}

// buildCompose assembles the file from the catalog: every active service is
// built from the environment, then published on the host or routed through
// the proxy according to its endpoints, and its named volumes are declared.
func buildCompose(e *Environment) composeFile {
	var file composeFile
	declared := map[string]struct{}{}

	for _, svc := range e.activeServices() {
		spec := svc.build(e, svc)
		publishOrRoute(&spec, e, svc)
		file.Services = file.Services.set(svc.Name, spec)

		for _, volume := range namedVolumes(spec) {
			if _, ok := declared[volume]; ok {
				continue
			}
			declared[volume] = struct{}{}
			file.Volumes = file.Volumes.set(volume, struct{}{})
		}
	}

	// In proxy mode every routed service joins the shared external network
	// Traefik also runs on, declared here so compose does not try to create it.
	if e.proxy != nil {
		file.Networks = yamlMap[composeExternalNetwork]{}.
			set(e.proxy.NetworkName, composeExternalNetwork{External: true})
	}

	return file
}

// namedVolumes returns the named (non bind-mount) volumes a service mounts,
// which the compose file has to declare at the top level.
func namedVolumes(spec composeService) []string {
	var names []string
	for _, mount := range spec.Volumes {
		source, _, _ := strings.Cut(mount, ":")
		if source == "" || strings.HasPrefix(source, ".") || strings.HasPrefix(source, "/") {
			continue
		}
		names = append(names, source)
	}

	return names
}
