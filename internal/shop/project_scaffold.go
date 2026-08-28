package shop

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/shopware/shopware-cli/logging"
)

const (
	CINone   = "none"
	CIGitHub = "github"
	CIGitLab = "gitlab"
)

//go:embed project_scaffold_static/deploy.php
var deployerTemplate string

//go:embed project_scaffold_static/github-ci.yml
var githubCITemplate string

//go:embed project_scaffold_static/github-deploy.yml
var githubDeployTemplate string

//go:embed project_scaffold_static/gitlab-ci.yml.tmpl
var gitlabCITemplate string

//go:embed project_scaffold_static/shopware-paas-application.yaml
var shopwarePaasAppTemplate string

//go:embed project_scaffold_static/Dockerfile.tmpl
var dockerfileTemplate string

//go:embed project_scaffold_static/dockerignore
var dockerignoreContent string

// ShopwareProjectScaffold describes the files and directories of a new
// Shopware project. CLI concerns such as prompting, tracking, and detecting
// local tools intentionally live outside this type.
type ShopwareProjectScaffold struct {
	ProjectFolder       string
	Version             string
	DeploymentMethod    string
	CISystem            string
	PHPVersion          string
	UseDocker           bool
	UseElasticsearch    bool
	UseAMQP             bool
	NoAudit             bool
	SymfonyCLIInstalled bool
}

// Normalize applies project defaults and deployment-specific requirements.
func (s *ShopwareProjectScaffold) Normalize() {
	if s.DeploymentMethod == "" {
		s.DeploymentMethod = DeploymentNone
	}
	if s.CISystem == "" {
		s.CISystem = CINone
	}
	if s.DeploymentMethod == DeploymentShopwarePaaS {
		s.UseElasticsearch = true
	}
	if s.DeploymentMethod == DeploymentContainer && s.PHPVersion == "" {
		s.PHPVersion = SupportedPHPVersions[len(SupportedPHPVersions)-1]
	}
}

// Validate ensures the scaffold options and target folder are safe to use.
func (s ShopwareProjectScaffold) Validate() error {
	if err := ValidateProjectFolder(s.ProjectFolder); err != nil {
		return err
	}
	if s.Version == "" {
		return errors.New("project version must not be empty")
	}
	if err := ValidateDeploymentMethod(s.DeploymentMethod); err != nil {
		return err
	}

	return ValidateCISystem(s.CISystem)
}

// Scaffold creates the initial project structure without installing its
// Composer dependencies.
func (s *ShopwareProjectScaffold) Scaffold(ctx context.Context) error {
	s.Normalize()
	if err := s.Validate(); err != nil {
		return err
	}

	if err := os.MkdirAll(s.ProjectFolder, os.ModePerm); err != nil {
		return err
	}

	logging.FromContext(ctx).Infof("Setting up Shopware %s", s.Version)

	if err := s.WriteComposerJson(ctx); err != nil {
		return err
	}

	envContent, err := EnvFileContent(s.UseDocker, s.ProjectFolder)
	if err != nil {
		return err
	}

	files := []struct {
		path    string
		content string
	}{
		{path: ".env", content: envContent},
		{path: ".env.local", content: envLocalContent(s.UseDocker)},
		{path: ".gitignore", content: "/.idea\n/.shopware-cli\n/vendor"},
	}
	for _, file := range files {
		if err := os.WriteFile(filepath.Join(s.ProjectFolder, file.path), []byte(file.content), os.ModePerm); err != nil {
			return err
		}
	}

	for _, path := range []string{
		filepath.Join("custom", "plugins"),
		filepath.Join("custom", "static-plugins"),
	} {
		if err := os.MkdirAll(filepath.Join(s.ProjectFolder, path), os.ModePerm); err != nil {
			return err
		}
	}

	if !s.UseDocker && s.SymfonyCLIInstalled {
		if err := os.WriteFile(filepath.Join(s.ProjectFolder, "php.ini"), []byte("memory_limit=512M"), os.ModePerm); err != nil {
			return err
		}
	}

	if err := s.setupDeployment(ctx); err != nil {
		return err
	}

	return setupCI(ctx, s.ProjectFolder, s.CISystem, s.DeploymentMethod)
}

// WriteComposerJson (re)generates the project's composer.json from the
// scaffold options, e.g. to disable composer's audit blocking after the
// initial scaffold has been written.
func (s *ShopwareProjectScaffold) WriteComposerJson(ctx context.Context) error {
	composerJSON, err := GenerateComposerJson(ctx, ComposerJsonOptions{
		Version:          s.Version,
		RC:               strings.Contains(s.Version, "rc"),
		UseElasticsearch: s.UseElasticsearch,
		UseAMQP:          s.UseAMQP,
		NoAudit:          s.NoAudit,
		DeploymentMethod: s.DeploymentMethod,
	})
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(s.ProjectFolder, "composer.json"), []byte(composerJSON), os.ModePerm)
}

// EnvLocalDockerContent is the initial .env.local of a Docker project. It is
// what switches the containers into the dev environment: the committed .env
// says APP_ENV=prod and the docker-dev image sets no environment of its own.
const EnvLocalDockerContent = "APP_ENV=dev\n"

func envLocalContent(useDocker bool) string {
	if useDocker {
		return EnvLocalDockerContent
	}

	return ""
}

func (s ShopwareProjectScaffold) setupDeployment(ctx context.Context) error {
	switch s.DeploymentMethod {
	case DeploymentDeployer:
		return os.WriteFile(filepath.Join(s.ProjectFolder, "deploy.php"), []byte(deployerTemplate), os.ModePerm)
	case DeploymentShopwarePaaS:
		return os.WriteFile(filepath.Join(s.ProjectFolder, "application.yaml"), []byte(shopwarePaasAppTemplate), os.ModePerm)
	case DeploymentContainer:
		return s.writeDockerfile(ctx)
	default:
		return nil
	}
}

// writeDockerfile generates the production Dockerfile and its .dockerignore,
// with both image stages pinned to the PHP the composer.lock was resolved with.
func (s ShopwareProjectScaffold) writeDockerfile(ctx context.Context) error {
	tmpl, err := template.New("dockerfile").Parse(dockerfileTemplate)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, struct{ PHPVersion string }{PHPVersion: s.PHPVersion}); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(s.ProjectFolder, "Dockerfile"), buf.Bytes(), os.ModePerm); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(s.ProjectFolder, ".dockerignore"), []byte(dockerignoreContent), os.ModePerm); err != nil {
		return err
	}

	logging.FromContext(ctx).Infof("Created Dockerfile for PHP %s", s.PHPVersion)

	return nil
}

func setupCI(ctx context.Context, projectFolder, ciSystem, deploymentMethod string) error {
	switch ciSystem {
	case CIGitHub:
		if err := os.MkdirAll(filepath.Join(projectFolder, ".github", "workflows"), os.ModePerm); err != nil {
			return err
		}

		ciPath := filepath.Join(".github", "workflows", "ci.yml")
		if err := os.WriteFile(filepath.Join(projectFolder, ciPath), []byte(githubCITemplate), os.ModePerm); err != nil {
			return err
		}
		logging.FromContext(ctx).Infof("Created CI template %s", ciPath)

		if deploymentMethod == DeploymentDeployer {
			deployPath := filepath.Join(".github", "workflows", "deploy.yml")
			if err := os.WriteFile(filepath.Join(projectFolder, deployPath), []byte(githubDeployTemplate), os.ModePerm); err != nil {
				return err
			}
			logging.FromContext(ctx).Infof("Created CI template %s", deployPath)
		}

	case CIGitLab:
		tmpl, err := template.New("gitlab-ci").Parse(gitlabCITemplate)
		if err != nil {
			return err
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, struct{ Deployer bool }{Deployer: deploymentMethod == DeploymentDeployer}); err != nil {
			return err
		}

		ciPath := ".gitlab-ci.yml"
		if err := os.WriteFile(filepath.Join(projectFolder, ciPath), buf.Bytes(), os.ModePerm); err != nil {
			return err
		}
		logging.FromContext(ctx).Infof("Created CI template %s", ciPath)
	}

	return nil
}
