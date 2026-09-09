package verifier

import (
	"strings"
	"sync"

	"github.com/shopware/shopware-cli/internal/validation"
)

type Check struct {
	Results    []validation.CheckResult `json:"results"`
	mutex      sync.Mutex
	sourceRoot string
}

func NewCheck() *Check {
	return &Check{
		Results: []validation.CheckResult{},
	}
}

// SetSourceRoot configures the analysis root used to rewrite finding paths
// into stable, input-relative locations before they are stored or reported.
func (c *Check) SetSourceRoot(root string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.sourceRoot = validation.ResolveSourceRoot(root)
}

func (c *Check) AddResult(result validation.CheckResult) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.sourceRoot != "" {
		result = validation.NormalizeResult(result, c.sourceRoot)
	}
	c.Results = append(c.Results, result)
}

func (c *Check) HasErrors() bool {
	for _, r := range c.Results {
		if r.Severity == validation.SeverityError {
			return true
		}
	}

	return false
}

func (c *Check) GetResults() []validation.CheckResult {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.Results
}

func (c *Check) RemoveByIdentifier(ignores []validation.ToolConfigIgnore) validation.Check {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	filtered := make([]validation.CheckResult, 0)
	for _, r := range c.Results {
		shouldKeep := true
		for _, ignore := range ignores {
			// Only ignore all matches when identifier is the only field specified
			if ignore.Identifier != "" && ignore.Path == "" && ignore.Message == "" {
				if validation.IdentifierMatches(r.Identifier, ignore.Identifier) {
					shouldKeep = false
					break
				}
			}

			// If path is specified with identifier (but no message), match both
			if ignore.Identifier != "" && ignore.Path != "" && ignore.Message == "" {
				if validation.IdentifierMatches(r.Identifier, ignore.Identifier) && c.samePath(r.Path, ignore.Path) {
					shouldKeep = false
					break
				}
			}

			// If identifier and message are specified (path is optional), match all specified fields
			if ignore.Identifier != "" && ignore.Message != "" {
				if validation.IdentifierMatches(r.Identifier, ignore.Identifier) && strings.Contains(r.Message, ignore.Message) && (ignore.Path == "" || c.samePath(r.Path, ignore.Path)) {
					shouldKeep = false
					break
				}
			}

			// Handle message-based ignores (when no identifier is specified)
			if ignore.Identifier == "" && ignore.Message != "" && strings.Contains(r.Message, ignore.Message) && (c.samePath(r.Path, ignore.Path) || ignore.Path == "") {
				shouldKeep = false
				break
			}
		}
		if shouldKeep {
			filtered = append(filtered, r)
		}
	}
	c.Results = filtered

	return c
}

func (c *Check) samePath(resultPath, ignorePath string) bool {
	if resultPath == ignorePath {
		return true
	}
	if ignorePath == "" || c.sourceRoot == "" {
		return false
	}

	return validation.NormalizeSourcePath(resultPath, c.sourceRoot) == validation.NormalizeSourcePath(ignorePath, c.sourceRoot)
}
