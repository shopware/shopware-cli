package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/logging"
)

type GitCommit struct {
	Hash    string
	Message string
}

func runGit(ctx context.Context, repo string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repo

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("cannot run git: %w, %s", err, output)
	}

	if cmd.ProcessState.ExitCode() != 0 {
		return "", fmt.Errorf("cannot run git: %s", string(output))
	}

	gitOuput := string(output)
	return strings.Trim(gitOuput, " "), nil
}

func getPreviousTag(ctx context.Context, currentTag, repo string) (string, error) {
	previousVersion := os.Getenv("SHOPWARE_CLI_PREVIOUS_TAG")
	if previousVersion != "" {
		return previousVersion, nil
	}

	tags, err := runGit(ctx, repo, "tag", "--list")
	if err != nil {
		return "", fmt.Errorf("failed to run git command: %w", err)
	}

	// direct tag match
	tagsArray := strings.Split(tags, "\n")

	var tagList []*version.Version

	for _, tag := range tagsArray {
		v, err := version.NewVersion(tag)
		if err != nil {
			continue
		}

		tagList = append(tagList, v)
	}

	currentVersion := version.Must(version.NewVersion(currentTag))

	sort.Sort(sort.Reverse(version.Collection(tagList)))

	// same major version
	currentMajor := currentVersion.Segments()[0]
	for _, tag := range tagList {
		if tag.Segments()[0] == currentMajor && tag.LessThan(currentVersion) {
			return tag.String(), nil
		}
	}

	// Look at previous major version
	for _, tag := range tagList {
		if tag.Segments()[0] == currentMajor-1 {
			return tag.String(), nil
		}
	}

	commits, err := runGit(ctx, repo, "log", "--pretty=format:%h", "--no-merges")
	if err != nil {
		return "", fmt.Errorf("cannot get previous tag: %w", err)
	}

	commitsArray := strings.Split(commits, "\n")

	// if no tag was found, return the first commit
	return commitsArray[len(commitsArray)-1], nil
}

func GetCommits(ctx context.Context, currentVersion, repo string) ([]GitCommit, error) {
	if err := unshallowRepository(ctx, repo); err != nil {
		return nil, err
	}

	currentTag, err := getTagForVersion(ctx, currentVersion, repo)
	if err != nil {
		return nil, err
	}

	logging.FromContext(ctx).Debugf("Current tag: %s", currentTag)

	previousTag, err := getPreviousTag(ctx, currentTag, repo)
	if err != nil {
		return nil, err
	}

	logging.FromContext(ctx).Debugf("Previous tag: %s", previousTag)
	logging.FromContext(ctx).Debugf("Diffing %s..HEAD", previousTag)

	commits, err := runGit(ctx, repo, "log", "--pretty=format:%h|%s", previousTag+"..HEAD", "--no-merges")
	if err != nil {
		return nil, fmt.Errorf("cannot get commits: %w", err)
	}

	if commits == "" {
		return []GitCommit{}, nil
	}

	commitsArray := strings.Split(commits, "\n")
	gitCommits := make([]GitCommit, len(commitsArray))

	for commit := range commitsArray {
		splitCommit := strings.Split(commitsArray[commit], "|")
		gitCommits[commit] = GitCommit{
			Hash:    splitCommit[0],
			Message: strings.Join(splitCommit[1:], "|"),
		}
	}

	return gitCommits, nil
}

func getTagForVersion(ctx context.Context, version, repo string) (string, error) {
	version = strings.TrimPrefix(version, "v")

	tags, err := runGit(ctx, repo, "tag", "--list")
	if err != nil {
		return "", fmt.Errorf("failed to run git command: %w", err)
	}

	// direct tag match
	tagsArray := strings.Split(tags, "\n")
	for _, tag := range tagsArray {
		if tag == version {
			return tag, nil
		}
	}

	// tag prefix match
	for _, tag := range tagsArray {
		tag = strings.TrimPrefix(tag, "v")
		if strings.HasPrefix(tag, version) {
			return tag, nil
		}
	}

	return version, nil
}

func GetPublicVCSURL(ctx context.Context, repo string) (string, error) {
	origin, err := runGit(ctx, repo, "config", "--get", "remote.origin.url")
	if err != nil {
		return "", fmt.Errorf("failed to run git command: %w", err)
	}

	origin = strings.Trim(origin, "\n")

	switch {
	case strings.HasPrefix(origin, "https://github.com/"):
		origin = strings.TrimSuffix(origin, ".git")

		return fmt.Sprintf("%s/commit", origin), nil
	case strings.HasPrefix(origin, "git@github.com:"):
		origin = origin[15:]
		origin = strings.TrimSuffix(origin, ".git")

		return fmt.Sprintf("https://github.com/%s/commit", origin), nil
	case os.Getenv("CI_PROJECT_URL") != "":
		return fmt.Sprintf("%s/-/commit", os.Getenv("CI_PROJECT_URL")), nil
	}

	return "", fmt.Errorf("unsupported vcs provider")
}

func unshallowRepository(ctx context.Context, repo string) error {
	if _, err := os.Stat(path.Join(repo, ".git", "shallow")); os.IsNotExist(err) {
		return nil
	}

	_, err := runGit(ctx, repo, "fetch", "--unshallow")

	return err
}
