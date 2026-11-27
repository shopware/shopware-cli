package runtime_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

func TestExitErrorNamesTerminationWithoutCode(t *testing.T) {
	assert.Equal(t, "process exited with code 7", (&runtime.ExitError{Code: 7}).Error())
	assert.Equal(t, "process was terminated without an exit code", (&runtime.ExitError{Code: -1}).Error())
	assert.Contains(t, (&runtime.ExitError{Code: -1, Signal: "SIGKILL"}).Error(), "SIGKILL")
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(data []byte) (int, error) { return f(data) }

func TestSameWriter(t *testing.T) {
	var shared bytes.Buffer
	uncomparable := writerFunc(func(data []byte) (int, error) { return len(data), nil })

	assert.True(t, runtime.SameWriter(nil, nil))
	assert.True(t, runtime.SameWriter(&shared, &shared))
	assert.False(t, runtime.SameWriter(&shared, &bytes.Buffer{}))
	assert.False(t, runtime.SameWriter(uncomparable, uncomparable), "uncomparable writers must not panic")
}

func TestJoinErrorsKeepsCausesOnOneLine(t *testing.T) {
	assert.NoError(t, runtime.JoinErrors(nil, nil))

	exit := &runtime.ExitError{Code: 1}

	err := runtime.JoinErrors(context.Canceled, nil, exit)

	assert.Equal(t, "context canceled; process exited with code 1", err.Error())
	assert.ErrorIs(t, err, context.Canceled)
	assert.ErrorIs(t, err, exit)
}
