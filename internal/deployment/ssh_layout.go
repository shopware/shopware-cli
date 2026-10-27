package deployment

import (
	_ "embed"
	"errors"
	"path"
	"slices"
	"strings"
)

//go:embed ssh_layout.php
var sshLayoutScript string

// withSSHDeploymentLayout prepends shared layout validation to a remote PHP script.
func withSSHDeploymentLayout(script string) string {
	return sshLayoutScript + "\n" + strings.TrimPrefix(script, "<?php\n")
}

// The configured project directory is the active symlink. Its parent holds
// releases, shared data and deployment metadata, regardless of the link's name.
func (s *SSH) deploymentRoot() (string, error) {
	current := path.Clean(s.directory)
	if !path.IsAbs(current) || path.Dir(current) == "/" ||
		strings.Contains(s.directory, "\\") ||
		strings.ContainsFunc(s.directory, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return "", errors.New("cannot resolve SSH deployment layout: ssh.directory must be an absolute path inside a deployment directory")
	}
	name := path.Base(current)
	if slices.Contains([]string{"releases", "shared", "runtime", ".shopware-cli"}, name) ||
		strings.HasPrefix(name, ".current-") {
		return "", errors.New("cannot resolve SSH deployment layout: ssh.directory conflicts with reserved deployment storage")
	}
	return path.Dir(current), nil
}

// activeLinkName returns the configured active release symlink name.
func (s *SSH) activeLinkName() string {
	return path.Base(path.Clean(s.directory))
}
