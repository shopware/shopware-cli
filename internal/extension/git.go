package extension

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/shopware/shopware-cli/internal/archiver"
)

func gitTagOrBranchOfFolder(ctx context.Context, source string) (string, error) {
	// Prefer a tag that points exactly at HEAD. This keeps the resulting filename
	// in sync with the actually checked-out commit (e.g. when a CI pipeline builds
	// a specific tag in detached-HEAD state). Picking the newest tag in the repo
	// regardless of HEAD would package the wrong tree (see issue #1116 / #753).
	tagCmd := exec.CommandContext(ctx, "git", "-C", source, "tag", "--points-at", "HEAD", "--sort=-creatordate")

	stdout, err := tagCmd.Output()
	if err != nil {
		return "", fmt.Errorf("cannot determine the git tag or branch of %s: %w", source, withGitStderr(err))
	}

	versions := strings.Split(string(stdout), "\n")

	if len(versions) > 0 && len(versions[0]) > 0 {
		return versions[0], nil
	}

	branchCmd := exec.CommandContext(ctx, "git", "-C", source, "rev-parse", "--abbrev-ref", "HEAD")

	stdout, err = branchCmd.Output()

	if err != nil {
		return "", fmt.Errorf("cannot determine the git tag or branch of %s: %w", source, withGitStderr(err))
	}

	return strings.Trim(strings.TrimLeft(string(stdout), "* "), "\n"), nil
}

func GitCopyFolder(ctx context.Context, source, target, commitHash string) (string, error) {
	var err error
	if commitHash == "" {
		commitHash, err = gitTagOrBranchOfFolder(ctx, source)

		if err != nil {
			return "", err
		}
	}

	archiveCmd := exec.CommandContext(ctx, "git", "-C", source, "archive", commitHash, "--format=zip")

	stdout, err := archiveCmd.Output()
	if err != nil {
		return "", fmt.Errorf("cannot archive %s: %w", commitHash, withGitStderr(err))
	}

	zipReader, err := zip.NewReader(bytes.NewReader(stdout), int64(len(stdout)))
	if err != nil {
		return "", fmt.Errorf("cannot open the zip file produced by git archive: %w", err)
	}

	err = archiver.Unzip(zipReader, target)
	if err != nil {
		return "", fmt.Errorf("cannot unzip the zip archive: %w", err)
	}

	return commitHash, err
}

// withGitStderr appends git's own stderr to an exec error, which otherwise only reads "exit status 128".
func withGitStderr(err error) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
		return fmt.Errorf("%w: %s", err, strings.ReplaceAll(strings.TrimSpace(string(exitErr.Stderr)), "\n", "; "))
	}

	return err
}
