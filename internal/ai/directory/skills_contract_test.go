package directory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const skillsDir = "../../../skills"

var (
	semver      = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$`)
	reportHash  = regexp.MustCompile(`(?m)^skill_sha256: ([0-9a-f]{64})$`)
	versionLine = regexp.MustCompile(`(?m)^\s+version:.*\n`)
	// Files a versioned skill may contain; anything else fails the contract.
	allowedEntries = []string{"SKILL.md", "CHANGELOG.md", "evals", "references", "scripts", "assets"}
	// Directories whose files are skill instructions: changing them needs a new
	// version and a new evaluation report.
	instructionDirs = []string{"references", "scripts", "assets"}
)

type skillFile struct {
	Name    string
	Version string
	// VersionQuoted is false when YAML would read the version as a number.
	VersionQuoted bool
}

func readSkill(t *testing.T, path string) skillFile {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	parts := strings.SplitN(string(content), "---\n", 3)
	require.Len(t, parts, 3, "%s must start with YAML frontmatter", path)

	var fm struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
		Metadata    struct {
			Version yaml.Node `yaml:"version"`
		} `yaml:"metadata"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(parts[1]), &fm))

	v := fm.Metadata.Version

	return skillFile{
		Name:          fm.Name,
		Version:       v.Value,
		VersionQuoted: v.Style == yaml.DoubleQuotedStyle || v.Style == yaml.SingleQuotedStyle,
	}
}

// instructionHash hashes SKILL.md (without its version line) and the files under references/, scripts/
// and assets/: the content an evaluation report was run against.
func instructionHash(t *testing.T, root string) string {
	t.Helper()

	files := []string{"SKILL.md"}
	for _, dir := range instructionDirs {
		_ = filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				rel, _ := filepath.Rel(root, path)
				files = append(files, filepath.ToSlash(rel))
			}

			return nil
		})
	}
	slices.Sort(files)

	h := sha256.New()
	for _, f := range files {
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		require.NoError(t, err)
		if f == "SKILL.md" {
			// The version line in the frontmatter changes with every release;
			// the instructions don't. Only that one line is left out.
			if parts := strings.SplitN(string(content), "---\n", 3); len(parts) == 3 {
				if loc := versionLine.FindStringIndex(parts[1]); loc != nil {
					parts[1] = parts[1][:loc[0]] + parts[1][loc[1]:]
				}
				content = []byte(strings.Join(parts, "---\n"))
			}
		}
		h.Write([]byte(f + "\x00"))
		h.Write(content)
		h.Write([]byte("\x00"))
	}

	return hex.EncodeToString(h.Sum(nil))
}

// Every skill the directory installs from this repository must exist under
// skills/ with a matching name, so `ai add` never points at a missing folder.
func TestDirectorySkillsFromThisRepositoryExist(t *testing.T) {
	for _, entry := range Load().Integrations {
		if entry.Type != TypeSkill || entry.Delivery.Repository != "shopware/shopware-cli" {
			continue
		}

		skill := readSkill(t, filepath.Join(skillsDir, entry.Name, "SKILL.md"))
		assert.Equal(t, entry.Name, skill.Name)
	}
}

type evalSuite struct {
	SchemaVersion int `json:"schema_version"`
	Scenarios     []struct {
		ID       string `json:"id"`
		Prompt   string `json:"prompt"`
		Criteria []struct {
			ID        string `json:"id"`
			Criterion string `json:"criterion"`
		} `json:"criteria"`
	} `json:"scenarios"`
}

// Skills with metadata.version are shipped as products (see
// docs/AGENT_SKILLS.md#contributor-skills): these are the deterministic gates.
// The version bump on change is checked against the pull request's base by
// TestChangedSkillsBumpTheirVersion (the versions job in agent-skills.yml).
func TestVersionedSkillsFollowTheProductContract(t *testing.T) {
	dirs, err := os.ReadDir(skillsDir)
	require.NoError(t, err)

	agentSkillsDoc, err := os.ReadFile("../../../docs/AGENT_SKILLS.md")
	require.NoError(t, err)

	for _, dir := range dirs {
		root := filepath.Join(skillsDir, dir.Name())
		skillPath := filepath.Join(root, "SKILL.md")
		skill := readSkill(t, skillPath)
		if skill.Version == "" {
			continue
		}

		t.Run(dir.Name(), func(t *testing.T) {
			// Structure and metadata. Name, folder and description length are
			// checked by skills-ref in agent-skills.yml; relative links by the
			// Markdown lint (MD057).
			assert.True(t, skill.VersionQuoted, "metadata.version must be a quoted string")
			assert.Regexp(t, semver, skill.Version, "metadata.version must be SemVer")

			body, err := os.ReadFile(skillPath)
			require.NoError(t, err)
			assert.LessOrEqual(t, strings.Count(string(body), "\n"), 500, "SKILL.md must stay under 500 lines")

			entries, err := os.ReadDir(root)
			require.NoError(t, err)
			for _, e := range entries {
				assert.Contains(t, allowedEntries, e.Name(), "file not allowed in a versioned skill")
			}

			// Registry: the AI directory and the docs list the skill.
			_, inDirectory := Load().Get(skill.Name)
			assert.True(t, inDirectory, "add the skill to internal/ai/directory/integrations.go")
			assert.Contains(t, string(agentSkillsDoc), "`"+skill.Name+"`", "list the skill in docs/AGENT_SKILLS.md")

			// Changelog entry for the current version.
			changelog, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
			require.NoError(t, err)
			assert.Contains(t, string(changelog), "## "+skill.Version+"\n", "CHANGELOG.md needs an entry for %s", skill.Version)

			// Evaluation suite: atomic, non-duplicated expectations.
			raw, err := os.ReadFile(filepath.Join(root, "evals", "scenarios.json"))
			require.NoError(t, err)
			var suite evalSuite
			require.NoError(t, json.Unmarshal(raw, &suite))
			assert.Equal(t, 1, suite.SchemaVersion)
			require.NotEmpty(t, suite.Scenarios)

			ids, prompts := map[string]bool{}, map[string]bool{}
			for _, s := range suite.Scenarios {
				assert.False(t, ids[s.ID], "duplicate scenario id %s", s.ID)
				assert.False(t, prompts[s.Prompt], "duplicate prompt in %s", s.ID)
				ids[s.ID], prompts[s.Prompt] = true, true
				assert.NotEmpty(t, s.Criteria, s.ID)

				criteria := map[string]bool{}
				for _, c := range s.Criteria {
					assert.False(t, criteria[c.Criterion], "duplicate criterion in %s", s.ID)
					criteria[c.Criterion] = true
				}
			}

			// Evaluation report for this version, run against the current
			// instructions.
			reportPath := filepath.Join(root, "evals", "reports", skill.Version+".md")
			report, err := os.ReadFile(reportPath)
			require.NoError(t, err, "add an evaluation report at evals/reports/%s.md", skill.Version)
			m := reportHash.FindSubmatch(report)
			require.NotNil(t, m, "the report needs a skill_sha256 line")
			assert.Equal(t, instructionHash(t, root), string(m[1]),
				"the skill changed after its evaluation report; re-run the evaluation and update skill_sha256")
		})
	}
}
