package extension

import (
	"context"
	"os"
	"path"
	"runtime"
	"sync"

	"github.com/shopware/shopware-cli/internal/npm"
	"github.com/shopware/shopware-cli/logging"
)

type npmInstallJob struct {
	npmPath             string
	additionalNpmParams []string
	additionalText      string
}

type npmInstallResult struct {
	nodeModulesPath string
	err             error
}

func InstallNodeModulesOfConfigs(ctx context.Context, cfgs ExtensionAssetConfig, force bool) ([]string, error) {
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

			if !force && npm.NodeModulesExists(npmPath) {
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
	for range numWorkers {
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
	npmPackage, err := npm.ReadPackage(job.npmPath)
	if err != nil {
		return npmInstallResult{err: err}
	}

	logging.FromContext(ctx).Infof("Installing npm dependencies in %s %s\n", job.npmPath, job.additionalText)

	if err := npm.InstallDependencies(ctx, job.npmPath, npmPackage, job.additionalNpmParams...); err != nil {
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
