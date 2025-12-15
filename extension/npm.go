package extension

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/shopware/shopware-cli/logging"
)

func nodeModulesExists(root string) bool {
	if _, err := os.Stat(path.Join(root, "node_modules")); err == nil {
		return true
	}

	return false
}

type npmInstallJob struct {
	npmPath             string
	additionalNpmParams []string
	additionalText      string
	useNpmCi            bool
}

type npmInstallResult struct {
	nodeModulesPath string
	err             error
}

func InstallNodeModulesOfConfigs(ctx context.Context, cfgs ExtensionAssetConfig, force bool, useNpmCi bool) ([]string, error) {
	// Collect all npm install jobs
	jobs := make([]npmInstallJob, 0)

	addedJobs := make(map[string]bool)

	// Install shared node_modules between admin and storefront
	for _, entry := range cfgs {
		additionalNpmParameters := []string{}

		if entry.NpmStrict {
			additionalNpmParameters = []string{"--production"}
		}

		for _, possibleNodePath := range entry.getPossibleNodePaths() {
			npmPath := path.Dir(possibleNodePath)

			if !force && nodeModulesExists(npmPath) {
				continue
			}

			additionalText := ""
			if !entry.NpmStrict {
				additionalText = " (consider enabling npm_strict mode, to install only production relevant dependencies)"
			}

			if !addedJobs[npmPath] {
				addedJobs[npmPath] = true
			} else {
				continue
			}

			jobs = append(jobs, npmInstallJob{
				npmPath:             npmPath,
				additionalNpmParams: additionalNpmParameters,
				additionalText:      additionalText,
				useNpmCi:            useNpmCi,
			})
		}
	}

	if len(jobs) == 0 {
		return []string{}, nil
	}

	// Set up worker pool with number of CPU cores
	numWorkers := runtime.NumCPU()
	jobChan := make(chan npmInstallJob, len(jobs))
	resultChan := make(chan npmInstallResult, len(jobs))

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobChan {
				result := processNpmInstallJob(ctx, job)
				resultChan <- result
			}
		}()
	}

	// Send jobs to workers
	for _, job := range jobs {
		jobChan <- job
	}
	close(jobChan)

	// Wait for all workers to finish
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	paths := make([]string, 0)
	for result := range resultChan {
		if result.err != nil {
			return nil, result.err
		}
		if result.nodeModulesPath != "" {
			paths = append(paths, result.nodeModulesPath)
		}
	}

	return paths, nil
}

func processNpmInstallJob(ctx context.Context, job npmInstallJob) npmInstallResult {
	npmPackage, err := getNpmPackage(job.npmPath)
	if err != nil {
		return npmInstallResult{err: err}
	}

	logging.FromContext(ctx).Infof("Installing npm dependencies in %s %s\n", job.npmPath, job.additionalText)

	if err := InstallNPMDependencies(ctx, job.npmPath, npmPackage, job.useNpmCi, job.additionalNpmParams...); err != nil {
		return npmInstallResult{err: err}
	}

	return npmInstallResult{
		nodeModulesPath: path.Join(job.npmPath, "node_modules"),
	}
}

func deletePaths(ctx context.Context, nodeModulesPaths ...string) {
	for _, nodeModulesPath := range nodeModulesPaths {
		if err := os.RemoveAll(nodeModulesPath); err != nil {
			logging.FromContext(ctx).Errorf("Failed to remove path %s: %s", nodeModulesPath, err.Error())
			return
		}
	}
}

func npmRunBuild(ctx context.Context, path string, buildCmd string, buildEnvVariables []string) error {
	npmBuildCmd := exec.CommandContext(ctx, "npm", "--prefix", path, "run", buildCmd) //nolint:gosec
	npmBuildCmd.Env = os.Environ()
	npmBuildCmd.Env = append(npmBuildCmd.Env, buildEnvVariables...)
	npmBuildCmd.Stdout = os.Stdout
	npmBuildCmd.Stderr = os.Stderr

	if err := npmBuildCmd.Run(); err != nil {
		return err
	}

	return nil
}

func InstallNPMDependencies(ctx context.Context, path string, packageJsonData NpmPackage, useNpmCi bool, additionalParams ...string) error {
	isProductionMode := false

	for _, param := range additionalParams {
		if param == "--production" {
			isProductionMode = true
		}
	}

	if isProductionMode && len(packageJsonData.Dependencies) == 0 {
		return nil
	}

	// Use npm ci if requested and package-lock.json exists
	npmCommand := "install"
	if useNpmCi {
		if _, err := os.Stat(filepath.Join(path, "package-lock.json")); err == nil {
			npmCommand = "ci"
		}
	}

	installCmd := exec.CommandContext(ctx, "npm", npmCommand, "--no-audit", "--no-fund", "--prefer-offline", "--loglevel=error")
	installCmd.Args = append(installCmd.Args, additionalParams...)
	installCmd.Dir = path
	installCmd.Env = os.Environ()
	installCmd.Env = append(installCmd.Env, "PUPPETEER_SKIP_DOWNLOAD=1", "NPM_CONFIG_ENGINE_STRICT=false", "NPM_CONFIG_FUND=false", "NPM_CONFIG_AUDIT=false", "NPM_CONFIG_UPDATE_NOTIFIER=false")

	combinedOutput, err := installCmd.CombinedOutput()
	if err != nil {
		logging.FromContext(context.Background()).Errorf("npm %s failed in %s: %s", npmCommand, path, string(combinedOutput))
		return fmt.Errorf("installing dependencies for %s failed with error: %w", path, err)
	}

	return nil
}

func getNpmPackage(root string) (NpmPackage, error) {
	packageJsonFile, err := os.ReadFile(path.Join(root, "package.json"))
	if err != nil {
		return NpmPackage{}, err
	}

	var packageJsonData NpmPackage
	if err := json.Unmarshal(packageJsonFile, &packageJsonData); err != nil {
		return NpmPackage{}, err
	}
	return packageJsonData, nil
}

func doesPackageJsonContainsPackageInDev(packageJsonData NpmPackage, packageName string) bool {
	if _, ok := packageJsonData.DevDependencies[packageName]; ok {
		return true
	}

	return false
}
