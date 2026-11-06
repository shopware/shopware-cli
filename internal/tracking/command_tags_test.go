package tracking

import (
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// resetCommandTags empties the collector before and after a test.
func resetCommandTags(t *testing.T) {
	t.Helper()
	reset := func() {
		commandTagsMu.Lock()
		defer commandTagsMu.Unlock()
		clear(commandTags)
	}
	reset()
	t.Cleanup(reset)
}

func TestCommandTags(t *testing.T) {
	resetCommandTags(t)

	SetCommandTag(TagExtensionCount, "3")
	SetCommandTag(TagExtensionCount, "4")

	tags := CommandTags()
	assert.Equal(t, map[string]string{TagExtensionCount: "4"}, tags)

	tags[TagExtensionCount] = "5"
	assert.Equal(t, "4", CommandTags()[TagExtensionCount], "callers get a copy")
}

func TestSetCommandTagConcurrently(t *testing.T) {
	resetCommandTags(t)

	var group sync.WaitGroup
	for i := range 3 {
		group.Go(func() {
			SetCommandTag(strconv.Itoa(i), "set")
		})
	}
	group.Wait()

	assert.Len(t, CommandTags(), 3)
}
