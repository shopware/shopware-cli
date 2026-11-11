package verifier

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/shopware/shopware-cli/internal/orchestration/container"
	"github.com/shopware/shopware-cli/internal/orchestration/executor"
	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
	containerruntime "github.com/shopware/shopware-cli/internal/orchestration/runtime/container"
)

var qaCLIVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z][0-9A-Za-z.-]*)?$`)
var qaPHPVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)

// ContainerQARequest describes a complete QA invocation. Flags must already use
// container paths for any path-valued arguments.
type ContainerQARequest struct {
	// Group selects project or extension commands.
	Group string
	// Operation is validate, fix or format.
	Operation string
	// CLIVersion is the running CLI release version.
	CLIVersion string
	// PHPVersion selects the image's PHP major.minor version.
	PHPVersion string
	// Root is the absolute host directory mounted as /workspace.
	Root string
	// Flags preserves QA selection and reporting options using target paths.
	Flags []string
	// User selects the container user; empty uses the image default.
	User string
	// Capture controls retained output independently of forwarded streams.
	Capture runtime.CaptureMode
}

// RunContainerQA runs a complete QA command using the supplied container runner.
// The caller selects container execution and resolves path-valued flags first.
func RunContainerQA(ctx context.Context, runner container.Runner, request ContainerQARequest, streams runtime.Options) (runtime.Result, error) {
	fail := func(err error) (runtime.Result, error) {
		return runtime.Result{ExitCode: -1}, &runtime.ExecutionError{Stage: runtime.StageStart, Err: err}
	}
	if request.Group != "project" && request.Group != "extension" {
		return fail(fmt.Errorf("unsupported QA group %q", request.Group))
	}
	if request.Operation != "validate" && request.Operation != "fix" && request.Operation != "format" {
		return fail(fmt.Errorf("unsupported QA operation %q", request.Operation))
	}
	version := strings.TrimPrefix(request.CLIVersion, "v")
	if !qaCLIVersion.MatchString(version) || !qaPHPVersion.MatchString(request.PHPVersion) {
		return fail(errors.New("invalid CLI or PHP version"))
	}
	if !filepath.IsAbs(request.Root) {
		return fail(errors.New("workspace root must be absolute"))
	}
	info, err := os.Stat(request.Root)
	if err != nil {
		return fail(fmt.Errorf("cannot access workspace root: %w", err))
	}
	if !info.IsDir() {
		return fail(errors.New("workspace root is not a directory"))
	}
	workspace, err := runtime.NewWorkspace(request.Root, "/workspace")
	if err != nil {
		return fail(err)
	}
	backend := containerruntime.New(runner, container.RunOptions{
		Image:  "ghcr.io/shopware/shopware-cli:" + version + "-php-" + request.PHPVersion,
		Mounts: []container.Mount{{Source: workspace.SourceRoot(), Target: workspace.TargetRoot()}},
		User:   request.User,
	})
	args := append([]string{request.Group, request.Operation, workspace.TargetRoot()}, request.Flags...)
	return executor.New(backend).WithDir(workspace.TargetRoot()).
		Command("shopware-cli", args...).
		WithEnv(map[string]string{"SHOPWARE_CLI_TOOLING_CONTEXT": "1"}).
		WithCapture(request.Capture).Run(ctx, streams)
}
