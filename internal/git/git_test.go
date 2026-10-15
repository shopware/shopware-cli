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
	c.Env = workingTreeEnvironment()

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

func TestIsInsideWorkingTree(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	prepareRepository(t, repo)
	assert.NoError(t, os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("ignored/\n"), 0o644))
	runCommand(t, repo, "add", ".gitignore")
	runCommand(t, repo, "commit", "-m", "initial commit")

	nested := filepath.Join(repo, "custom", "plugins", "Example")
	ignored := filepath.Join(repo, "ignored")
	for _, dir := range []string{nested, ignored} {
		assert.NoError(t, os.MkdirAll(dir, 0o755))
	}
	assert.NoError(t, os.WriteFile(filepath.Join(nested, "untracked.txt"), []byte("local work"), 0o644))

	worktree := filepath.Join(t.TempDir(), "worktree")
	runCommand(t, repo, "worktree", "add", "-b", "linked", worktree)
	link := filepath.Join(t.TempDir(), "linked-extension")
	assert.NoError(t, os.Symlink(nested, link))

	runCommand(t, repo, "-c", "protocol.file.allow=always", "submodule", "add", repo, "module")
	outside := t.TempDir()
	externalLink := filepath.Join(repo, "external")
	assert.NoError(t, os.Symlink(outside, externalLink))

	bare := t.TempDir()
	runCommand(t, bare, "init", "--bare")
	empty := t.TempDir()
	assert.NoError(t, os.Mkdir(filepath.Join(empty, ".git"), 0o755))

	for _, tc := range []struct {
		name string
		path string
		want bool
	}{
		{"repository root", repo, true},
		{"nested untracked extension", nested, true},
		{"ignored directory", ignored, true},
		{"linked worktree", worktree, true},
		{"submodule", filepath.Join(repo, "module"), true},
		{"symlink outside repository", externalLink, false},
		{"symlink to extension", link, true},
		{"bare repository", bare, false},
		{"empty git directory", empty, false},
		{"outside git", t.TempDir(), false},
		{"missing path", filepath.Join(t.TempDir(), "missing"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			inside, err := IsInsideWorkingTree(t.Context(), tc.path)
			assert.NoError(t, err)
			assert.Equal(t, tc.want, inside)
		})
	}
}

func TestIsInsideWorkingTreeWithoutGit(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	inside, err := IsInsideWorkingTree(t.Context(), t.TempDir())
	assert.Error(t, err)
	assert.False(t, inside)
}

func TestWorkingTreeChecksIgnoreRepositoryEnvironment(t *testing.T) {
	ambient := t.TempDir()
	prepareRepository(t, ambient)
	assert.NoError(t, os.WriteFile(filepath.Join(ambient, "ambient.txt"), []byte("ambient work"), 0o644))

	for _, tc := range []struct {
		name     string
		dir      string
		workTree string
	}{
		{"git dir", filepath.Join(ambient, ".git"), ""},
		{"work tree", "", ambient},
		{"both", filepath.Join(ambient, ".git"), ambient},
		{"invalid locations", filepath.Join(ambient, "missing"), filepath.Join(ambient, "missing")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.dir != "" {
				t.Setenv("GIT_DIR", tc.dir)
			}
			if tc.workTree != "" {
				t.Setenv("GIT_WORK_TREE", tc.workTree)
			}

			repo := t.TempDir()
			prepareRepository(t, repo)
			_, err := os.Stat(filepath.Join(repo, ".git", "HEAD"))
			assert.NoError(t, err)
			inside, err := IsInsideWorkingTree(t.Context(), repo)
			assert.NoError(t, err)
			assert.True(t, inside)

			dirty, isRepo, err := IsWorkingTreeDirty(t.Context(), repo)
			assert.NoError(t, err)
			assert.True(t, isRepo)
			assert.False(t, dirty)

			assert.NoError(t, os.WriteFile(filepath.Join(repo, "target.txt"), []byte("target work"), 0o644))
			dirty, isRepo, err = IsWorkingTreeDirty(t.Context(), repo)
			assert.NoError(t, err)
			assert.True(t, isRepo)
			assert.True(t, dirty)

			outside := t.TempDir()
			inside, err = IsInsideWorkingTree(t.Context(), outside)
			assert.NoError(t, err)
			assert.False(t, inside)
			dirty, isRepo, err = IsWorkingTreeDirty(t.Context(), outside)
			assert.NoError(t, err)
			assert.False(t, isRepo)
			assert.False(t, dirty)
		})
	}
}
