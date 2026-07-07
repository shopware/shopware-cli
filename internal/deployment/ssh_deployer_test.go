package deployment

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/shop"
)

// fakeConnection records executed commands and answers them with canned responses.
type fakeConnection struct {
	commands  []string
	streamed  []string
	responses map[string]string
	failures  map[string]error
	closed    bool
}

func (f *fakeConnection) Run(_ context.Context, command string) (string, error) {
	f.commands = append(f.commands, command)

	for prefix, err := range f.failures {
		if strings.Contains(command, prefix) {
			return "", err
		}
	}

	for prefix, response := range f.responses {
		if strings.Contains(command, prefix) {
			return response, nil
		}
	}

	return "", nil
}

func (f *fakeConnection) Stream(_ context.Context, command string, stdin io.Reader) error {
	_, _ = io.Copy(io.Discard, stdin)
	f.streamed = append(f.streamed, command)
	return nil
}

func (f *fakeConnection) Close() error {
	f.closed = true
	return nil
}

func newTestDeployer(t *testing.T, conn *fakeConnection, cfg *shop.EnvironmentDeployment) *sshDeployer {
	t.Helper()

	return newMultiHostTestDeployer(t, []deployHost{{name: "server1", conn: conn}}, cfg)
}

func newMultiHostTestDeployer(t *testing.T, hosts []deployHost, cfg *shop.EnvironmentDeployment) *sshDeployer {
	t.Helper()

	if cfg == nil {
		cfg = &shop.EnvironmentDeployment{}
	}

	return &sshDeployer{
		projectRoot: t.TempDir(),
		deployPath:  "/var/www/shop",
		config:      cfg,
		hosts:       hosts,
		now:         func() time.Time { return time.Date(2026, 7, 7, 12, 30, 45, 0, time.UTC) },
		runLocal:    func(context.Context, string, string) error { return nil },
	}
}

func commandsContaining(commands []string, needle string) []string {
	var matches []string
	for _, command := range commands {
		if strings.Contains(command, needle) {
			matches = append(matches, command)
		}
	}
	return matches
}

func TestDeployCreatesReleaseAndSwitchesSymlink(t *testing.T) {
	conn := &fakeConnection{
		responses: map[string]string{
			"ls -1 '/var/www/shop/releases' 2>/dev/null": "20260707123045\n",
			"readlink": "/var/www/shop/releases/20260707123045\n",
		},
	}

	deployer := newTestDeployer(t, conn, nil)

	err := deployer.Deploy(t.Context(), Options{})
	assert.NoError(t, err)

	// structure setup
	assert.Contains(t, conn.commands, "mkdir -p '/var/www/shop/releases' '/var/www/shop/shared'")
	assert.Contains(t, conn.commands, "mkdir -p '/var/www/shop/releases/20260707123045'")

	// upload streamed into the release directory
	assert.Equal(t, []string{"tar -xzpf - -C '/var/www/shop/releases/20260707123045'"}, conn.streamed)

	// atomic symlink switch
	assert.Contains(t, conn.commands, "ln -sfn '/var/www/shop/releases/20260707123045' '/var/www/shop/current.tmp' && mv -fT '/var/www/shop/current.tmp' '/var/www/shop/current'")
}

func TestDeployLinksDefaultSharedPaths(t *testing.T) {
	conn := &fakeConnection{}
	deployer := newTestDeployer(t, conn, nil)

	err := deployer.Deploy(t.Context(), Options{})
	assert.NoError(t, err)

	links := commandsContaining(conn.commands, "ln -sfn '/var/www/shop/shared/")

	assert.Len(t, links, len(defaultSharedDirs)+len(defaultSharedFiles))
	assert.NotEmpty(t, commandsContaining(links, "'/var/www/shop/shared/public/media'"))
	assert.NotEmpty(t, commandsContaining(links, "'/var/www/shop/shared/.env'"))
}

func TestDeployRunsConfiguredHooks(t *testing.T) {
	var localHooks []string

	conn := &fakeConnection{}
	deployer := newTestDeployer(t, conn, &shop.EnvironmentDeployment{
		Hooks: shop.EnvironmentDeploymentHooks{
			Build:      []string{"shopware-cli project ci ."},
			PreSwitch:  []string{"php bin/console system:setup"},
			PostSwitch: []string{"php bin/console cache:pool:clear cache.http"},
		},
	})
	deployer.runLocal = func(_ context.Context, _ string, command string) error {
		localHooks = append(localHooks, command)
		return nil
	}

	err := deployer.Deploy(t.Context(), Options{})
	assert.NoError(t, err)

	assert.Equal(t, []string{"shopware-cli project ci ."}, localHooks)
	assert.Contains(t, conn.commands, "cd '/var/www/shop/releases/20260707123045' && { php bin/console system:setup; }")
	assert.Contains(t, conn.commands, "cd '/var/www/shop/current' && { php bin/console cache:pool:clear cache.http; }")

	// no deployment-helper detection when hooks are configured
	assert.Empty(t, commandsContaining(conn.commands, "shopware-deployment-helper"))
}

func TestDeploySkipsBuildHooks(t *testing.T) {
	conn := &fakeConnection{}
	deployer := newTestDeployer(t, conn, &shop.EnvironmentDeployment{
		Hooks: shop.EnvironmentDeploymentHooks{Build: []string{"exit 1"}},
	})
	deployer.runLocal = func(context.Context, string, string) error {
		t.Fatal("build hook should not run")
		return nil
	}

	err := deployer.Deploy(t.Context(), Options{SkipBuildHooks: true})
	assert.NoError(t, err)
}

func TestDeployUsesDeploymentHelperWhenPresent(t *testing.T) {
	conn := &fakeConnection{
		responses: map[string]string{
			"test -f '/var/www/shop/releases/20260707123045/vendor/bin/shopware-deployment-helper'": "found\n",
		},
	}
	deployer := newTestDeployer(t, conn, nil)

	err := deployer.Deploy(t.Context(), Options{})
	assert.NoError(t, err)

	assert.Contains(t, conn.commands, "cd '/var/www/shop/releases/20260707123045' && { vendor/bin/shopware-deployment-helper run; }")
}

func TestDeployRefusesUnmanagedCurrent(t *testing.T) {
	conn := &fakeConnection{
		failures: map[string]error{
			"[ ! -e '/var/www/shop/current' ]": fmt.Errorf("exit 1"),
		},
	}
	deployer := newTestDeployer(t, conn, nil)

	err := deployer.Deploy(t.Context(), Options{})
	assert.ErrorContains(t, err, "not a symlink")
	assert.Empty(t, conn.streamed)
}

func TestDeployCleansUpOldReleases(t *testing.T) {
	conn := &fakeConnection{
		responses: map[string]string{
			"ls -1 '/var/www/shop/releases' 2>/dev/null": "1\n2\n3\n4\n",
			"readlink": "/var/www/shop/releases/4\n",
		},
	}
	deployer := newTestDeployer(t, conn, &shop.EnvironmentDeployment{KeepReleases: 2})

	err := deployer.Deploy(t.Context(), Options{})
	assert.NoError(t, err)

	var removals []string
	for _, command := range conn.commands {
		if strings.HasPrefix(command, "rm -rf ") {
			removals = append(removals, command)
		}
	}

	assert.ElementsMatch(t, []string{
		"rm -rf '/var/www/shop/releases/1'",
		"rm -rf '/var/www/shop/releases/2'",
	}, removals)
}

func TestReleases(t *testing.T) {
	conn := &fakeConnection{
		responses: map[string]string{
			"ls -1 '/var/www/shop/releases' 2>/dev/null": "3\n1\n2\n",
			"readlink":    "/var/www/shop/releases/2\n",
			"BAD_RELEASE": "/var/www/shop/releases/3/BAD_RELEASE\n",
		},
	}
	deployer := newTestDeployer(t, conn, nil)

	releases, err := deployer.Releases(t.Context())
	assert.NoError(t, err)

	assert.Equal(t, []HostReleases{
		{
			Host: "server1",
			Releases: []Release{
				{Name: "1"},
				{Name: "2", Active: true},
				{Name: "3", Bad: true},
			},
		},
	}, releases)
}

func TestRollbackToPreviousRelease(t *testing.T) {
	conn := &fakeConnection{
		responses: map[string]string{
			"ls -1 '/var/www/shop/releases' 2>/dev/null": "1\n2\n3\n",
			"readlink": "/var/www/shop/releases/3\n",
		},
	}
	deployer := newTestDeployer(t, conn, nil)

	err := deployer.Rollback(t.Context(), "")
	assert.NoError(t, err)

	assert.Contains(t, conn.commands, "ln -sfn '/var/www/shop/releases/2' '/var/www/shop/current.tmp' && mv -fT '/var/www/shop/current.tmp' '/var/www/shop/current'")
	assert.Contains(t, conn.commands, "touch '/var/www/shop/releases/3/BAD_RELEASE'")
}

func TestRollbackSkipsBadReleases(t *testing.T) {
	conn := &fakeConnection{
		responses: map[string]string{
			"ls -1 '/var/www/shop/releases' 2>/dev/null": "1\n2\n3\n",
			"readlink":    "/var/www/shop/releases/3\n",
			"BAD_RELEASE": "/var/www/shop/releases/2/BAD_RELEASE\n",
		},
	}
	deployer := newTestDeployer(t, conn, nil)

	err := deployer.Rollback(t.Context(), "")
	assert.NoError(t, err)

	assert.Contains(t, conn.commands, "ln -sfn '/var/www/shop/releases/1' '/var/www/shop/current.tmp' && mv -fT '/var/www/shop/current.tmp' '/var/www/shop/current'")
}

func TestRollbackToExplicitRelease(t *testing.T) {
	conn := &fakeConnection{
		responses: map[string]string{
			"ls -1 '/var/www/shop/releases' 2>/dev/null": "1\n2\n3\n",
			"readlink": "/var/www/shop/releases/3\n",
		},
	}
	deployer := newTestDeployer(t, conn, nil)

	err := deployer.Rollback(t.Context(), "1")
	assert.NoError(t, err)

	assert.Contains(t, conn.commands, "ln -sfn '/var/www/shop/releases/1' '/var/www/shop/current.tmp' && mv -fT '/var/www/shop/current.tmp' '/var/www/shop/current'")
}

func TestRollbackErrors(t *testing.T) {
	cases := []struct {
		name     string
		target   string
		releases string
		active   string
		expected string
	}{
		{
			name:     "no active release",
			releases: "1\n2\n",
			active:   "",
			expected: "cannot determine the active release",
		},
		{
			name:     "no earlier release",
			releases: "1\n",
			active:   "/var/www/shop/releases/1\n",
			expected: "no earlier release available",
		},
		{
			name:     "already active",
			target:   "2",
			releases: "1\n2\n",
			active:   "/var/www/shop/releases/2\n",
			expected: "already active",
		},
		{
			name:     "unknown release",
			target:   "5",
			releases: "1\n2\n",
			active:   "/var/www/shop/releases/2\n",
			expected: "does not exist",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			conn := &fakeConnection{
				responses: map[string]string{
					"ls -1 '/var/www/shop/releases' 2>/dev/null": tc.releases,
					"readlink": tc.active,
				},
			}
			deployer := newTestDeployer(t, conn, nil)

			err := deployer.Rollback(t.Context(), tc.target)
			assert.ErrorContains(t, err, tc.expected)
		})
	}
}

func TestNewDeployerRequiresSupportedType(t *testing.T) {
	_, err := NewDeployer(t.TempDir(), &shop.EnvironmentConfig{Type: "local"}, &shop.Config{})
	assert.ErrorContains(t, err, "does not support deployments")
}

func TestNewSSHDeployerRequiresDeploymentPath(t *testing.T) {
	_, err := newSSHDeployer(t.TempDir(), &shop.EnvironmentConfig{Type: "ssh"}, &shop.Config{})
	assert.ErrorContains(t, err, "deployment.path")
}

func TestMultiHostDeployRunsOnAllHosts(t *testing.T) {
	conn1 := &fakeConnection{}
	conn2 := &fakeConnection{}
	deployer := newMultiHostTestDeployer(t, []deployHost{
		{name: "web1", conn: conn1},
		{name: "web2", conn: conn2},
	}, &shop.EnvironmentDeployment{
		Hooks: shop.EnvironmentDeploymentHooks{
			PreSwitch:  []string{"echo setup"},
			PostSwitch: []string{"echo done"},
		},
	})

	err := deployer.Deploy(t.Context(), Options{})
	assert.NoError(t, err)

	for _, conn := range []*fakeConnection{conn1, conn2} {
		assert.Equal(t, []string{"tar -xzpf - -C '/var/www/shop/releases/20260707123045'"}, conn.streamed)
		assert.Contains(t, conn.commands, "cd '/var/www/shop/releases/20260707123045' && { echo setup; }")
		assert.Contains(t, conn.commands, "ln -sfn '/var/www/shop/releases/20260707123045' '/var/www/shop/current.tmp' && mv -fT '/var/www/shop/current.tmp' '/var/www/shop/current'")
		assert.Contains(t, conn.commands, "cd '/var/www/shop/current' && { echo done; }")
	}
}

func TestMultiHostDeployAbortsBeforeSwitchWhenHostFails(t *testing.T) {
	conn1 := &fakeConnection{}
	conn2 := &fakeConnection{
		failures: map[string]error{"echo setup": fmt.Errorf("exit 1")},
	}
	deployer := newMultiHostTestDeployer(t, []deployHost{
		{name: "web1", conn: conn1},
		{name: "web2", conn: conn2},
	}, &shop.EnvironmentDeployment{
		Hooks: shop.EnvironmentDeploymentHooks{PreSwitch: []string{"echo setup"}},
	})

	err := deployer.Deploy(t.Context(), Options{})
	assert.ErrorContains(t, err, "host web2")

	// no host was switched, the running release stays untouched everywhere
	assert.Empty(t, commandsContaining(conn1.commands, "mv -fT"))
	assert.Empty(t, commandsContaining(conn2.commands, "mv -fT"))
}

func TestMultiHostRollbackRequiresReleaseOnAllHosts(t *testing.T) {
	conn1 := &fakeConnection{
		responses: map[string]string{
			"ls -1 '/var/www/shop/releases' 2>/dev/null": "1\n2\n",
			"readlink": "/var/www/shop/releases/2\n",
		},
	}
	conn2 := &fakeConnection{
		responses: map[string]string{
			"ls -1 '/var/www/shop/releases' 2>/dev/null": "2\n",
			"readlink": "/var/www/shop/releases/2\n",
		},
	}
	deployer := newMultiHostTestDeployer(t, []deployHost{
		{name: "web1", conn: conn1},
		{name: "web2", conn: conn2},
	}, nil)

	err := deployer.Rollback(t.Context(), "")
	assert.ErrorContains(t, err, "release 1 does not exist on host web2")

	assert.Empty(t, commandsContaining(conn1.commands, "mv -fT"))
	assert.Empty(t, commandsContaining(conn2.commands, "mv -fT"))
}

func TestMultiHostRollbackSwitchesAllHosts(t *testing.T) {
	responses := map[string]string{
		"ls -1 '/var/www/shop/releases' 2>/dev/null": "1\n2\n",
		"readlink": "/var/www/shop/releases/2\n",
	}
	conn1 := &fakeConnection{responses: responses}
	conn2 := &fakeConnection{responses: responses}
	deployer := newMultiHostTestDeployer(t, []deployHost{
		{name: "web1", conn: conn1},
		{name: "web2", conn: conn2},
	}, nil)

	err := deployer.Rollback(t.Context(), "")
	assert.NoError(t, err)

	for _, conn := range []*fakeConnection{conn1, conn2} {
		assert.Contains(t, conn.commands, "ln -sfn '/var/www/shop/releases/1' '/var/www/shop/current.tmp' && mv -fT '/var/www/shop/current.tmp' '/var/www/shop/current'")
		assert.Contains(t, conn.commands, "touch '/var/www/shop/releases/2/BAD_RELEASE'")
	}
}

func TestNewSSHDeployerRequiresHost(t *testing.T) {
	_, err := newSSHDeployer(t.TempDir(), &shop.EnvironmentConfig{
		Type:       "ssh",
		Deployment: &shop.EnvironmentDeployment{Path: "/var/www/shop"},
	}, &shop.Config{})
	assert.ErrorContains(t, err, "ssh.host")
}
