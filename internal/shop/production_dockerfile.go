package shop

import (
	"bytes"
	"text/template"
)

type ProductionDockerfileOptions struct {
	PHPVersion          string
	WithDevDependencies bool
}

// ProductionDockerfile is shared by project scaffolding and container packaging.
func ProductionDockerfile(opts ProductionDockerfileOptions) ([]byte, error) {
	if err := ValidatePHPVersion(opts.PHPVersion); err != nil {
		return nil, err
	}
	tmpl, err := template.New("dockerfile").Parse(dockerfileTemplate)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, opts); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func ProductionDockerignore() string {
	return dockerignoreContent
}
