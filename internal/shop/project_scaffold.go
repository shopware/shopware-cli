package shop

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
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

// ShopwareProjectScaffold describes the files and directories of a new
// Shopware project. CLI concerns such as prompting, tracking, and detecting
// local tools intentionally live outside this type.
type ShopwareProjectScaffold struct {
	ProjectFolder       string
	Version             string
	DeploymentMethod    string
	CISystem            string
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
}

// Validate ensures the scaffold options and target folder are safe to use.
func (s ShopwareProjectScaffold) Validate() error {
	if err := ValidateProjectFolder(s.ProjectFolder); err != nil {
		return err
	}
	if s.Version == "" {
		return fmt.Errorf("project version must not be empty")
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

	files := []struct {
		path    string
		content string
	}{
		{path: ".env"},
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

	if err := setupDeployment(s.ProjectFolder, s.DeploymentMethod); err != nil {
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

func envLocalContent(useDocker bool) string {
	if useDocker {
		return "APP_ENV=dev\n"
	}

	return ""
}

func setupDeployment(projectFolder, deploymentMethod string) error {
	switch deploymentMethod {
	case DeploymentDeployer:
		return os.WriteFile(filepath.Join(projectFolder, "deploy.php"), []byte(deployerTemplate), os.ModePerm)
	case DeploymentShopwarePaaS:
		return os.WriteFile(filepath.Join(projectFolder, "application.yaml"), []byte(shopwarePaasAppTemplate), os.ModePerm)
	default:
		return nil
	}
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
