package verifier

import (
	"strings"
	"sync"

	"github.com/shopware/shopware-cli/internal/validation"
)

type Check struct {
	Results []validation.CheckResult `json:"results"`
	mutex   sync.Mutex
}

func NewCheck() *Check {
	return &Check{
		Results: []validation.CheckResult{},
	}
}

func (c *Check) AddResult(result validation.CheckResult) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
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
				if r.Identifier == ignore.Identifier {
					shouldKeep = false
					break
				}
			}

			// If path is specified with identifier (but no message), match both
			if ignore.Identifier != "" && ignore.Path != "" && ignore.Message == "" {
				if r.Identifier == ignore.Identifier && r.Path == ignore.Path {
					shouldKeep = false
					break
				}
			}

			// Handle message-based ignores (when no identifier is specified)
			if ignore.Identifier == "" && ignore.Message != "" && strings.Contains(r.Message, ignore.Message) && (r.Path == ignore.Path || ignore.Path == "") {
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
