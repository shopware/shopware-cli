package tracking

import (
	"maps"
	"sync"
)

var (
	commandTagsMu sync.Mutex
	// commandTags collects the tags a command adds to its EventCommand. Like sessionID, there is one per CLI run.
	commandTags = map[string]string{}
)

// SetCommandTag adds a tag to the command event, whether the command succeeds or fails.
// Tags the command event sets itself, such as result, take precedence.
// It is safe for concurrent use.
func SetCommandTag(key, value string) {
	commandTagsMu.Lock()
	defer commandTagsMu.Unlock()
	commandTags[key] = value
}

// CommandTags returns a copy of the tags set with SetCommandTag.
func CommandTags() map[string]string {
	commandTagsMu.Lock()
	defer commandTagsMu.Unlock()
	return maps.Clone(commandTags)
}
