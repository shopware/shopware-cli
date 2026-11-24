package directory

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// firstVersion is the version every new versioned skill starts at.
const firstVersion = "0.0.1"

// validBump reports whether next is the next major, minor or patch version
// after prev, with lower parts reset to zero.
func validBump(prev, next string) bool {
	p, n := parseVersion(prev), parseVersion(next)
	if p == nil || n == nil {
		return false
	}

	switch {
	case n[0] == p[0]+1:
		return n[1] == 0 && n[2] == 0
	case n[0] == p[0] && n[1] == p[1]+1:
		return n[2] == 0
	default:
		return n[0] == p[0] && n[1] == p[1] && n[2] == p[2]+1
	}
}

func parseVersion(v string) []int {
	if !semver.MatchString(v) {
		return nil
	}

	parts := strings.Split(v, ".")
	out := make([]int, 3)
	for i, s := range parts {
		out[i], _ = strconv.Atoi(s)
	}

	return out
}

func TestValidBump(t *testing.T) {
	for _, c := range []struct {
		prev, next string
		ok         bool
	}{
		{"1.0.0", "1.0.1", true},
		{"1.4.2", "1.5.0", true},
		{"1.4.2", "2.0.0", true},
		{"1.0.0", "1.0.0", false},
		{"1.4.2", "1.5.2", false},
		{"1.4.2", "2.1.0", false},
		{"1.0.0", "1.0.2", false},
		{"1.0.0", "0.9.0", false},
		{"1.0.0", "1.0", false},
	} {
		assert.Equal(t, c.ok, validBump(c.prev, c.next), "%s -> %s", c.prev, c.next)
	}
}

// TestChangedSkillsBumpTheirVersion blocks a pull request that changes a
// versioned skill's instructions without a version bump, or without linking the
// skill's evaluation report in its description. CI sets SKILLS_BASE_REF (the
// pull request's base) and PR_BODY; locally the test is skipped.
func TestChangedSkillsBumpTheirVersion(t *testing.T) {
	base := os.Getenv("SKILLS_BASE_REF")
	if base == "" {
		t.Skip("SKILLS_BASE_REF not set")
	}
	body, checkBody := os.LookupEnv("PR_BODY")

	repo, err := filepath.Abs("../../..")
	require.NoError(t, err)

	git := func(args ...string) (string, error) {
		out, err := exec.CommandContext(t.Context(), "git", append([]string{"-C", repo}, args...)...).Output()
		return string(out), err
	}

	dirs, err := os.ReadDir(skillsDir)
	require.NoError(t, err)

	for _, dir := range dirs {
		name := dir.Name()
		head := readSkill(t, filepath.Join(skillsDir, name, "SKILL.md")).Version
		if head == "" {
			continue
		}

		paths := []string{"skills/" + name + "/SKILL.md"}
		for _, d := range instructionDirs {
			paths = append(paths, "skills/"+name+"/"+d)
		}
		changed, err := git(append([]string{"diff", "--name-only", base + "...HEAD", "--"}, paths...)...)
		require.NoError(t, err)
		if strings.TrimSpace(changed) == "" {
			continue
		}

		t.Run(name, func(t *testing.T) {
			prev := ""
			if old, err := git("show", base+":skills/"+name+"/SKILL.md"); err == nil {
				parts := strings.SplitN(old, "---\n", 3)
				if len(parts) == 3 {
					var fm struct {
						Metadata struct {
							Version string `yaml:"version"`
						} `yaml:"metadata"`
					}
					_ = yaml.Unmarshal([]byte(parts[1]), &fm)
					prev = fm.Metadata.Version
				}
			}

			if prev == "" {
				assert.Equal(t, firstVersion, head, "a new versioned skill starts at %s", firstVersion)
			} else {
				assert.True(t, validBump(prev, head), "instructions changed: bump metadata.version from %s (now %s)", prev, head)
			}

			if checkBody {
				report := "skills/" + name + "/evals/reports/" + head + ".md"
				assert.Contains(t, body, report, "link the evaluation report in the pull request description")
			}
		})
	}
}
