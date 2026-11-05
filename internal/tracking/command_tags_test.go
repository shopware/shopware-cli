package tracking

import (
	"context"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCommandTags(t *testing.T) {
	ctx := WithCommandTags(t.Context())
	// Commands receive a context derived from the one the root command created.
	commandCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	SetCommandTag(commandCtx, TagExtensionCount, "3")
	SetCommandTag(commandCtx, TagExtensionCount, "4")

	tags := CommandTags(ctx)
	assert.Equal(t, map[string]string{TagExtensionCount: "4"}, tags)

	tags[TagExtensionCount] = "5"
	assert.Equal(t, "4", CommandTags(ctx)[TagExtensionCount], "callers get a copy")
}

func TestCommandTagsWithoutCollector(t *testing.T) {
	SetCommandTag(t.Context(), TagExtensionCount, "3")

	assert.Empty(t, CommandTags(t.Context()))
}

func TestSetCommandTagConcurrently(t *testing.T) {
	ctx := WithCommandTags(t.Context())
	var group sync.WaitGroup
	for i := range 3 {
		group.Go(func() {
			SetCommandTag(ctx, strconv.Itoa(i), "set")
		})
	}
	group.Wait()

	assert.Len(t, CommandTags(ctx), 3)
}
