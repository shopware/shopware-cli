package tracking

import (
	"context"
	"maps"
	"sync"
)

type commandTagsKey struct{}

// commandTags collects the tags a command adds to its EventCommand.
type commandTags struct {
	mu   sync.Mutex
	tags map[string]string
}

// WithCommandTags returns a context that collects tags for the command event.
func WithCommandTags(ctx context.Context) context.Context {
	return context.WithValue(ctx, commandTagsKey{}, &commandTags{tags: map[string]string{}})
}

// SetCommandTag adds a tag to the command event, whether the command succeeds or fails.
// Tags the command event sets itself, such as result, take precedence.
// It is safe for concurrent use and does nothing on a context without WithCommandTags.
func SetCommandTag(ctx context.Context, key, value string) {
	collected, ok := ctx.Value(commandTagsKey{}).(*commandTags)
	if !ok {
		return
	}

	collected.mu.Lock()
	defer collected.mu.Unlock()
	collected.tags[key] = value
}

// CommandTags returns a copy of the tags set with SetCommandTag.
func CommandTags(ctx context.Context) map[string]string {
	collected, ok := ctx.Value(commandTagsKey{}).(*commandTags)
	if !ok {
		return nil
	}

	collected.mu.Lock()
	defer collected.mu.Unlock()
	return maps.Clone(collected.tags)
}
