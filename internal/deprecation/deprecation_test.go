package deprecation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMessageNamesReplacementWhenGiven(t *testing.T) {
	assert.Contains(t, Message("2027-02-10", "--format"), "--format")
	assert.NotContains(t, Message("2027-02-10", ""), "use")
}

func TestTrackRecordsEachIDOnceSorted(t *testing.T) {
	t.Cleanup(func() { used = nil })

	Track("flag.reporter")
	Track("command.zip")
	Track("flag.reporter")

	assert.Equal(t, []string{"command.zip", "flag.reporter"}, Used())
}
