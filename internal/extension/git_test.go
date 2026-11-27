package extension

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, string(out))
}

func writeComposerJSON(t *testing.T, dir, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "composer.json"), []byte(content), 0o644))
}

func TestGitCopyFolderArchivesHeadNotNewestTag(t *testing.T) {
	source := t.TempDir()
	ctx := t.Context()

	gitRun(t, source, "init")
	gitRun(t, source, "config", "commit.gpgsign", "false")
	gitRun(t, source, "config", "tag.gpgSign", "false")
	gitRun(t, source, "config", "tag.forceSignAnnotated", "false")

	// Older release 1.0.0
	writeComposerJSON(t, source, `{"version":"1.0.0"}`)
	gitRun(t, source, "add", ".")
	gitRun(t, source, "commit", "-m", "1.0.0")

	// Current release 2.0.0 on HEAD
	writeComposerJSON(t, source, `{"version":"2.0.0"}`)
	gitRun(t, source, "add", ".")
	gitRun(t, source, "commit", "-m", "2.0.0")
	gitRun(t, source, "tag", "-a", "-m", "2.0.0", "2.0.0")

	// Tag the older commit *after* the newer one, so a -creatordate sort over all
	// tags would surface 1.0.0 first - the regression from issue #1116.
	gitRun(t, source, "tag", "-a", "-m", "1.0.0", "1.0.0", "HEAD~1")

	target := t.TempDir()

	tag, err := GitCopyFolder(ctx, source, target, "")
	require.NoError(t, err)

	assert.Equal(t, "2.0.0", tag)

	content, err := os.ReadFile(filepath.Join(target, "composer.json"))
	require.NoError(t, err)
	assert.Equal(t, `{"version":"2.0.0"}`, string(content))
}

func TestGitCopyFolderFallsBackToBranchWithoutTagAtHead(t *testing.T) {
	source := t.TempDir()
	ctx := t.Context()

	gitRun(t, source, "init")
	gitRun(t, source, "config", "commit.gpgsign", "false")
	gitRun(t, source, "config", "tag.gpgSign", "false")
	gitRun(t, source, "config", "tag.forceSignAnnotated", "false")
	gitRun(t, source, "checkout", "-b", "main")

	writeComposerJSON(t, source, `{"version":"1.0.0"}`)
	gitRun(t, source, "add", ".")
	gitRun(t, source, "commit", "-m", "1.0.0")
	gitRun(t, source, "tag", "-a", "-m", "1.0.0", "1.0.0")

	// New unreleased commit on top of the tag - no tag points at HEAD.
	writeComposerJSON(t, source, `{"version":"1.0.1"}`)
	gitRun(t, source, "add", ".")
	gitRun(t, source, "commit", "-m", "wip")

	target := t.TempDir()

	tag, err := GitCopyFolder(ctx, source, target, "")
	require.NoError(t, err)

	assert.Equal(t, "main", tag)

	content, err := os.ReadFile(filepath.Join(target, "composer.json"))
	require.NoError(t, err)
	assert.Equal(t, `{"version":"1.0.1"}`, string(content))
}

func TestGitCopyFolderOutsideGitRepository(t *testing.T) {
	cases := []struct {
		name     string
		gitTrace string
	}{
		{name: "plain", gitTrace: ""},
		// Git writes trace lines to stderr before its error.
		{name: "with GIT_TRACE", gitTrace: "1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GIT_TRACE", tc.gitTrace)

			source := t.TempDir()
			writeComposerJSON(t, source, `{"version":"1.0.0"}`)

			_, err := GitCopyFolder(t.Context(), source, t.TempDir(), "")

			assert.ErrorIs(t, err, ErrNotGitRepository)
		})
	}
}

func TestGitCopyFolderKeepsOtherGitErrors(t *testing.T) {
	fakeGitDir := t.TempDir()
	script := "#!/bin/sh\necho \"fatal: detected dubious ownership in repository at '$2'\" >&2\nexit 128\n"
	require.NoError(t, os.WriteFile(filepath.Join(fakeGitDir, "git"), []byte(script), 0o755))
	t.Setenv("PATH", fakeGitDir)

	// The path contains the words Git uses for a missing repository.
	source := filepath.Join(t.TempDir(), "not a git repository")
	require.NoError(t, os.Mkdir(source, 0o755))

	_, err := GitCopyFolder(t.Context(), source, t.TempDir(), "")

	assert.NotErrorIs(t, err, ErrNotGitRepository)
	assert.ErrorContains(t, err, "dubious ownership")
}

func TestGitCopyFolderKeepsErrorForBrokenGitFile(t *testing.T) {
	// A submodule or worktree copied without its parent repository.
	source := t.TempDir()
	writeComposerJSON(t, source, `{"version":"1.0.0"}`)
	missing := filepath.Join(t.TempDir(), "missing")
	require.NoError(t, os.WriteFile(filepath.Join(source, ".git"), []byte("gitdir: "+missing+"\n"), 0o644))

	_, err := GitCopyFolder(t.Context(), source, t.TempDir(), "")

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotGitRepository)
}
