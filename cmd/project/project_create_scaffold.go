package project

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tracking"
	"github.com/shopware/shopware-cli/logging"
)

//go:embed static/deploy.php
var deployerTemplate string

//go:embed static/github-ci.yml
var githubCITemplate string

//go:embed static/github-deploy.yml
var githubDeployTemplate string

//go:embed static/gitlab-ci.yml.tmpl
var gitlabCITemplate string

//go:embed static/shopware-paas-application.yaml
var shopwarePaasAppTemplate string

func scaffoldProject(ctx context.Context, opts *createOptions, chosenVersion string) error {
	go tracking.Track(ctx, tracking.EventProjectCreate, map[string]string{
		tracking.TagVersion:           opts.selectedVersion,
		tracking.TagDeployment:        opts.selectedDeployment,
		tracking.TagCI:                opts.selectedCI,
		tracking.TagDocker:            fmt.Sprintf("%v", opts.useDocker),
		tracking.TagWithElasticsearch: fmt.Sprintf("%v", opts.withElasticsearch),
		tracking.TagWithAMQP:          fmt.Sprintf("%v", opts.withAMQP),
		tracking.TagInteractive:       fmt.Sprintf("%v", opts.interactive),
	})

	projectFolder := opts.projectFolder
	if projectFolder == "." {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("could not determine current directory: %w", err)
		}
		projectFolder = cwd
	}

	if err := os.MkdirAll(projectFolder, os.ModePerm); err != nil {
		return err
	}

	empty, err := system.IsDirEmpty(projectFolder)
	if err != nil {
		return err
	}
	if !empty {
		return fmt.Errorf("the folder %s exists already and is not empty", projectFolder)
	}

	if opts.projectFolder != "." {
		logging.FromContext(ctx).Infof("Setting up Shopware %s in %s", chosenVersion, opts.projectFolder)
	} else {
		logging.FromContext(ctx).Infof("Setting up Shopware %s in the current directory", chosenVersion)
	}

	composerJson, err := shop.GenerateComposerJson(ctx, shop.ComposerJsonOptions{
		Version:          chosenVersion,
		RC:               strings.Contains(chosenVersion, "rc"),
		UseElasticsearch: opts.withElasticsearch,
		UseAMQP:          opts.withAMQP,
		NoAudit:          opts.noAudit,
		DeploymentMethod: opts.selectedDeployment,
	})
	if err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(projectFolder, "composer.json"), []byte(composerJson), os.ModePerm); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(projectFolder, ".env"), []byte(""), os.ModePerm); err != nil {
		return err
	}

	envLocalContent := ""
	if opts.useDocker {
		envLocalContent += "APP_ENV=dev\n"
	}

	if err := os.WriteFile(filepath.Join(projectFolder, ".env.local"), []byte(envLocalContent), os.ModePerm); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(projectFolder, ".gitignore"), []byte("/.idea\n/vendor"), os.ModePerm); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Join(projectFolder, "custom", "plugins"), os.ModePerm); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Join(projectFolder, "custom", "static-plugins"), os.ModePerm); err != nil {
		return err
	}

	if !opts.useDocker && system.IsSymfonyCliInstalled() {
		if err := os.WriteFile(filepath.Join(projectFolder, "php.ini"), []byte("memory_limit=512M"), os.ModePerm); err != nil {
			return err
		}
	}

	if err := setupDeployment(projectFolder, opts.selectedDeployment); err != nil {
		return err
	}

	return setupCI(ctx, projectFolder, opts.selectedCI, opts.selectedDeployment)
}

func setupDeployment(projectFolder, deploymentMethod string) error {
	switch deploymentMethod {
	case shop.DeploymentDeployer:
		if err := os.WriteFile(filepath.Join(projectFolder, "deploy.php"), []byte(deployerTemplate), os.ModePerm); err != nil {
			return err
		}

	case shop.DeploymentShopwarePaaS:
		if err := os.WriteFile(filepath.Join(projectFolder, "application.yaml"), []byte(shopwarePaasAppTemplate), os.ModePerm); err != nil {
			return err
		}
	}

	return nil
}

func setupCI(ctx context.Context, projectFolder, ciSystem, deploymentMethod string) error {
	switch ciSystem {
	case ciGitHub:
		if err := os.MkdirAll(filepath.Join(projectFolder, ".github", "workflows"), os.ModePerm); err != nil {
			return err
		}
		ciPath := filepath.Join(".github", "workflows", "ci.yml")
		if err := os.WriteFile(filepath.Join(projectFolder, ciPath), []byte(githubCITemplate), os.ModePerm); err != nil {
			return err
		}
		logging.FromContext(ctx).Infof("Created CI template %s", ciPath)
		if deploymentMethod == shop.DeploymentDeployer {
			deployPath := filepath.Join(".github", "workflows", "deploy.yml")
			if err := os.WriteFile(filepath.Join(projectFolder, deployPath), []byte(githubDeployTemplate), os.ModePerm); err != nil {
				return err
			}
			logging.FromContext(ctx).Infof("Created CI template %s", deployPath)
		}

	case ciGitLab:
		tmpl, err := template.New("gitlab-ci").Parse(gitlabCITemplate)
		if err != nil {
			return err
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, struct{ Deployer bool }{Deployer: deploymentMethod == shop.DeploymentDeployer}); err != nil {
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
