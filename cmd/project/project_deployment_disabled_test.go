//go:build !deployment

package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeploymentCommandsDisabled(t *testing.T) {
	for _, cmd := range projectRootCmd.Commands() {
		assert.NotEqual(t, "deployment", cmd.Name(), "deployment commands must not be registered in normal builds")
	}
}
