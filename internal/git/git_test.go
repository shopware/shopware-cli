package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInvalidGitRepository(t *testing.T) {
	t.Parallel()
	repo := "invalid"
	ctx := t.Context()

	tag, err := getPreviousTag(ctx, "", repo)
	assert.Error(t, err)
	assert.Empty(t, tag)
}

func TestNoTags(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	prepareRepository(t, tmpDir)
	_ = os.WriteFile(filepath.Join(tmpDir, "a"), []byte(""), 0o644)
	runCommand(t, tmpDir, "add", "a")
	runCommand(t, tmpDir, "commit", "-m", "initial commit", "--no-verify", "--no-gpg-sign")

	tag, err := getPreviousTag(t.Context(), "1.0.0", tmpDir)
	assert.NoError(t, err)
	assert.NotEmpty(t, tag)

	currentTag, err := getTagForVersion(t.Context(), "1.0.0", tmpDir)
	assert.NoError(t, err)
	assert.Equal(t, "1.0.0", currentTag)

	commits, err := GetCommits(t.Context(), "1.0.0", tmpDir)
	assert.NoError(t, err)
	assert.Len(t, commits, 0)
}

func TestWithOneTagAndCommit(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	prepareRepository(t, tmpDir)
	_ = os.WriteFile(filepath.Join(tmpDir, "a"), []byte(""), 0o644)
	runCommand(t, tmpDir, "add", "a")
	runCommand(t, tmpDir, "commit", "-m", "initial commit", "--no-verify", "--no-gpg-sign")
	runCommand(t, tmpDir, "tag", "v1.0.0", "-m", "initial release")
	_ = os.WriteFile(filepath.Join(tmpDir, "b"), []byte(""), 0o644)
	runCommand(t, tmpDir, "add", "b")
	runCommand(t, tmpDir, "commit", "-m", "second commit", "--no-verify", "--no-gpg-sign")

	tag, err := getPreviousTag(t.Context(), "1.0.0", tmpDir)
	assert.NoError(t, err)
	assert.NotEqual(t, tag, "v1.0.0")

	currentTag, err := getTagForVersion(t.Context(), "1.0.0", tmpDir)
	assert.NoError(t, err)
	assert.Equal(t, "1.0.0", currentTag)

	commits, err := GetCommits(t.Context(), "1.0.0", tmpDir)
	assert.NoError(t, err)
	assert.Len(t, commits, 1)
	assert.Equal(t, commits[0].Message, "second commit")
}

func TestGetPublicVCSURL(t *testing.T) {
	tmpDir := t.TempDir()
	prepareRepository(t, tmpDir)

	url, err := GetPublicVCSURL(t.Context(), tmpDir)
	assert.Equal(t, "", url)
	assert.Error(t, err)

	runCommand(t, tmpDir, "remote", "add", "origin", "https://github.com/FriendsOfShopware/FroshTools.git")

	url, err = GetPublicVCSURL(t.Context(), tmpDir)
	assert.Equal(t, "https://github.com/FriendsOfShopware/FroshTools/commit", url)
	assert.NoError(t, err)

	runCommand(t, tmpDir, "remote", "set-url", "origin", "git@github.com:FriendsOfShopware/FroshTools.git")

	url, err = GetPublicVCSURL(t.Context(), tmpDir)
	assert.Equal(t, "https://github.com/FriendsOfShopware/FroshTools/commit", url)
	assert.NoError(t, err)

	runCommand(t, tmpDir, "remote", "set-url", "origin", "https://gitlab.com/xxx")
	t.Setenv("CI_PROJECT_URL", "https://example.com/gitlab-org/gitlab-foss")

	url, err = GetPublicVCSURL(t.Context(), tmpDir)
	assert.Equal(t, "https://example.com/gitlab-org/gitlab-foss/-/commit", url)
	assert.NoError(t, err)
}

func TestIsWorkingTreeDirty(t *testing.T) {
	t.Run("clean git working tree", func(t *testing.T) {
		tmpDir := t.TempDir()
		prepareRepository(t, tmpDir)

		dirty, isGitRepository, err := IsWorkingTreeDirty(t.Context(), tmpDir)

		assert.NoError(t, err)
		assert.True(t, isGitRepository)
		assert.False(t, dirty)
	})

	t.Run("dirty git working tree", func(t *testing.T) {
		tmpDir := t.TempDir()
		prepareRepository(t, tmpDir)
		assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "untracked.txt"), []byte("local work"), 0o644))

		dirty, isGitRepository, err := IsWorkingTreeDirty(t.Context(), tmpDir)

		assert.NoError(t, err)
		assert.True(t, isGitRepository)
		assert.True(t, dirty)
	})

	t.Run("not a git repository", func(t *testing.T) {
		dirty, isGitRepository, err := IsWorkingTreeDirty(t.Context(), t.TempDir())

		assert.NoError(t, err)
		assert.False(t, isGitRepository)
		assert.False(t, dirty)
	})
}

func runCommand(t *testing.T, tmpDir string, args ...string) {
	t.Helper()

	c := exec.CommandContext(t.Context(), "git", args...)
	c.Dir = tmpDir

	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %s", string(out))
	}
}

func prepareRepository(t *testing.T, tmpDir string) {
	t.Helper()

	runCommand(t, tmpDir, "init")
	runCommand(t, tmpDir, "config", "commit.gpgsign", "false")
	runCommand(t, tmpDir, "config", "tag.gpgsign", "false")
	runCommand(t, tmpDir, "config", "user.name", "test")
	runCommand(t, tmpDir, "config", "user.email", "test@test.de")
}

func TestIsRepositoryFalseForPlainDir(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	assert.False(t, IsRepository(t.Context(), tmpDir))
}

func TestIsRepositoryTrueForInitializedRepo(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	prepareRepository(t, tmpDir)
	assert.True(t, IsRepository(t.Context(), tmpDir))
}

func TestWorkingTreeStatusCleanRepo(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	prepareRepository(t, tmpDir)
	_ = os.WriteFile(filepath.Join(tmpDir, "a"), []byte("hi"), 0o644)
	runCommand(t, tmpDir, "add", "a")
	runCommand(t, tmpDir, "commit", "-m", "initial", "--no-verify", "--no-gpg-sign")

	lines, err := WorkingTreeStatus(t.Context(), tmpDir)
	assert.NoError(t, err)
	assert.Empty(t, lines, "freshly committed repo should be clean")
}

func TestWorkingTreeStatusReportsUntrackedAndModified(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	prepareRepository(t, tmpDir)
	_ = os.WriteFile(filepath.Join(tmpDir, "tracked.txt"), []byte("hi"), 0o644)
	runCommand(t, tmpDir, "add", "tracked.txt")
	runCommand(t, tmpDir, "commit", "-m", "initial", "--no-verify", "--no-gpg-sign")

	_ = os.WriteFile(filepath.Join(tmpDir, "tracked.txt"), []byte("modified"), 0o644)
	_ = os.WriteFile(filepath.Join(tmpDir, "untracked.txt"), []byte("new"), 0o644)

	lines, err := WorkingTreeStatus(t.Context(), tmpDir)
	assert.NoError(t, err)
	assert.Len(t, lines, 2)
}
