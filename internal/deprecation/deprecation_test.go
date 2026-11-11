package deprecation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMessageNamesReplacementWhenGiven(t *testing.T) {
	assert.Contains(t, Message("2027-02-10", "--format"), "--format")
	assert.NotContains(t, Message("2027-02-10", ""), "use")
}

func TestMessagePanicsOnInvalidDate(t *testing.T) {
	assert.Panics(t, func() { Message("10-02-2027", "") })
}

func TestTrackRecordsEachIDOnceSorted(t *testing.T) {
	t.Cleanup(func() { used = nil })

	Track("flag.extension.validate.reporter")
	Track("command.extension.zip")
	Track("flag.extension.validate.reporter")

	assert.Equal(t, []string{"command.extension.zip", "flag.extension.validate.reporter"}, Used())
}
