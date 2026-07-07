package projectupgrade

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/git"
)

// PreflightStatus is the outcome of a single preflight check.
type PreflightStatus int

const (
	PreflightOK PreflightStatus = iota
	PreflightFailed
	PreflightSkipped
)

// PreflightResult is the outcome of one environment/safety check shown in the
// wizard's preflight checklist.
type PreflightResult struct {
	// Label is the short check name shown in the checklist.
	Label string
	// Status is the check outcome.
	Status PreflightStatus
	// Detail is a short outcome note shown next to the label (e.g. "clean",
	// "skipped via --allow-dirty").
	Detail string
	// Explanation says why a failed check blocks the wizard and how to fix
	// it. Only rendered when Status is PreflightFailed.
	Explanation string
}

// PreflightBlocked reports whether any check failed. Failed checks block the
// wizard until they pass a recheck.
func PreflightBlocked(results []PreflightResult) bool {
	for _, r := range results {
		if r.Status == PreflightFailed {
			return true
		}
	}
	return false
}

// packagistPingURL is hit to verify the composer package repository is
// reachable. Overridable in tests.
var packagistPingURL = "https://repo.packagist.org/packages.json"

// preflightHTTPClient is used for reachability checks. Short timeout so a
// broken network fails the check quickly instead of hanging the wizard.
var preflightHTTPClient = &http.Client{Timeout: 10 * time.Second}

// RunPreflightChecks executes every preflight check and returns the results
// in display order. It never returns an error; failures are encoded in the
// results so the wizard can show them and offer a recheck.
func RunPreflightChecks(ctx context.Context, opts WizardOptions) []PreflightResult {
	return []PreflightResult{
		checkComposerLock(opts.ProjectRoot),
		checkGitClean(ctx, opts.ProjectRoot, opts.AllowDirty),
		checkComposerManagedPlugins(opts.ProjectRoot, opts.AllowNonComposer),
		checkEnvironmentRunning(ctx, opts.Executor),
		checkPackagistReachable(ctx),
	}
}

func checkComposerLock(projectRoot string) PreflightResult {
	r := PreflightResult{Label: "composer.lock present"}

	if _, err := os.Stat(filepath.Join(projectRoot, "composer.lock")); err != nil {
		r.Status = PreflightFailed
		r.Detail = "not found"
		r.Explanation = "The upgrade needs composer.lock to determine installed packages. Run `composer install` first."
		return r
	}

	r.Status = PreflightOK
	return r
}

func checkGitClean(ctx context.Context, projectRoot string, allowDirty bool) PreflightResult {
	r := PreflightResult{Label: "Git working tree clean"}

	if allowDirty {
		r.Status = PreflightSkipped
		r.Detail = "skipped via --allow-dirty"
		return r
	}

	dirty, isRepo, err := git.IsWorkingTreeDirty(ctx, projectRoot)
	if err != nil {
		r.Status = PreflightFailed
		r.Detail = "check failed"
		r.Explanation = fmt.Sprintf("Could not read the git working tree status: %v", err)
		return r
	}

	if !isRepo {
		r.Status = PreflightOK
		r.Detail = "not a git repository"
		return r
	}

	if dirty {
		r.Status = PreflightFailed
		r.Detail = "uncommitted changes"
		r.Explanation = "The upgrade rewrites composer.json and removes recipe-managed files. Commit or stash your changes so you can roll back, or rerun with --allow-dirty."
		return r
	}

	r.Status = PreflightOK
	r.Detail = "clean"
	return r
}

func checkComposerManagedPlugins(projectRoot string, allowNonComposer bool) PreflightResult {
	r := PreflightResult{Label: "Plugins managed by Composer"}

	if allowNonComposer {
		r.Status = PreflightSkipped
		r.Detail = "skipped via --allow-non-composer"
		return r
	}

	orphans, err := FindNonComposerPlugins(projectRoot)
	if err != nil {
		r.Status = PreflightFailed
		r.Detail = "check failed"
		r.Explanation = fmt.Sprintf("Could not scan custom/plugins: %v", err)
		return r
	}

	if len(orphans) > 0 {
		r.Status = PreflightFailed
		r.Detail = fmt.Sprintf("%d not tracked by composer", len(orphans))
		r.Explanation = fmt.Sprintf(
			"The upgrade can only bump composer-managed plugins, but these directories in custom/plugins/ are not tracked by composer: %s. Run `shopware-cli project autofix composer-plugins` to migrate them, or rerun with --allow-non-composer.",
			strings.Join(orphans, ", "),
		)
		return r
	}

	r.Status = PreflightOK
	return r
}

func checkEnvironmentRunning(ctx context.Context, exec executor.Executor) PreflightResult {
	r := PreflightResult{Label: "Web environment running"}

	if exec == nil {
		r.Status = PreflightSkipped
		r.Detail = "no executor"
		return r
	}

	running, err := exec.EnvironmentStatus(ctx)
	if errors.Is(err, executor.ErrNotSupported) {
		r.Status = PreflightOK
		r.Detail = exec.Type() + " (no managed services)"
		return r
	}
	if err != nil {
		r.Status = PreflightFailed
		r.Detail = "status unknown"
		r.Explanation = fmt.Sprintf("Could not determine whether the %s environment is running: %v", exec.Type(), err)
		return r
	}

	if !running {
		r.Status = PreflightFailed
		r.Detail = exec.Type() + " services stopped"
		r.Explanation = "The upgrade runs composer and the deployment helper inside your environment. Start it (e.g. `docker compose up -d`) and recheck."
		return r
	}

	r.Status = PreflightOK
	r.Detail = exec.Type() + " services running"
	return r
}

func checkPackagistReachable(ctx context.Context) PreflightResult {
	r := PreflightResult{Label: "Packagist reachable"}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, packagistPingURL, http.NoBody)
	if err != nil {
		r.Status = PreflightFailed
		r.Detail = "check failed"
		r.Explanation = err.Error()
		return r
	}
	req.Header.Set("User-Agent", "Shopware CLI")

	resp, err := preflightHTTPClient.Do(req)
	if err != nil {
		r.Status = PreflightFailed
		r.Detail = "unreachable"
		r.Explanation = "Composer needs to reach the package repository to resolve the upgrade. Check your network, proxy, or hosting firewall, then recheck. Details: " + err.Error()
		return r
	}
	_ = resp.Body.Close()

	if resp.StatusCode >= 500 {
		r.Status = PreflightFailed
		r.Detail = resp.Status
		r.Explanation = "The package repository responded with a server error. Try the recheck in a moment."
		return r
	}

	r.Status = PreflightOK
	return r
}
