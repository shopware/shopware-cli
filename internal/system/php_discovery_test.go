package system

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	phpdiscover "github.com/shyim/go-php-discover"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// canonicalPath resolves symlinks in path the same way discovery does, so
// assertions hold on platforms where t.TempDir() contains symlinks (macOS).
func canonicalPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	assert.NoError(t, err)
	return resolved
}

// versionSetChecker is a PHPVersionChecker accepting an explicit set of versions.
type versionSetChecker map[string]bool

func (c versionSetChecker) Check(phpVersion string) bool { return c[phpVersion] }
func (c versionSetChecker) String() string               { return "test constraint" }

func findInstallation(installations []PHPInstallation, binary string) *PHPInstallation {
	for i := range installations {
		if installations[i].Binary == binary {
			return &installations[i]
		}
	}
	return nil
}

func TestDiscoverPHPInstallationsUsesPATH(t *testing.T) {
	pathDir := t.TempDir()
	writeFakePHP(t, pathDir+"/php", "8.2.0")
	writeFakePHP(t, pathDir+"/php8.3", "8.3.0")
	writeFakePHP(t, pathDir+"/php-config", "8.3.0") // must be ignored
	writeFakePHP(t, pathDir+"/phpize", "8.3.0")     // must be ignored

	t.Setenv("PATH", pathDir)
	t.Setenv("PHP_BINARY", "")

	installations := DiscoverPHPInstallations(t.Context())

	require.NotNil(t, findInstallation(installations, canonicalPath(t, pathDir+"/php")))
	require.NotNil(t, findInstallation(installations, canonicalPath(t, pathDir+"/php8.3")))
	assert.Nil(t, findInstallation(installations, canonicalPath(t, pathDir+"/php-config")))
	assert.Nil(t, findInstallation(installations, canonicalPath(t, pathDir+"/phpize")))
}

func TestDiscoverPHPInstallationsPrefersPHPBinaryEnvFirst(t *testing.T) {
	pathDir := t.TempDir()
	writeFakePHP(t, pathDir+"/php", "8.2.0")

	binDir := t.TempDir()
	writeFakePHP(t, binDir+"/my-php", "8.3.0")

	t.Setenv("PATH", pathDir)
	t.Setenv("PHP_BINARY", binDir+"/my-php")

	installations := DiscoverPHPInstallations(t.Context())

	require.NotEmpty(t, installations)
	assert.Equal(t, PHPSourceEnv, installations[0].Source)
	assert.Equal(t, canonicalPath(t, binDir+"/my-php"), installations[0].Binary)
	assert.Equal(t, "8.3.0", installations[0].Version)
}

func TestDiscoverPHPInstallationsDedupesPHPBinaryWithPATH(t *testing.T) {
	pathDir := t.TempDir()
	writeFakePHP(t, pathDir+"/php", "8.3.7")

	t.Setenv("PATH", pathDir)
	t.Setenv("PHP_BINARY", pathDir+"/php")

	installations := DiscoverPHPInstallations(t.Context())

	matching := 0
	for _, installation := range installations {
		if installation.Binary == canonicalPath(t, pathDir+"/php") {
			matching++
			assert.Equal(t, PHPSourceEnv, installation.Source)
			assert.True(t, installation.Default)
		}
	}
	assert.Equal(t, 1, matching)
}

func TestDiscoverPHPInstallationsSortsNewestFirstKeepingPHPBinaryFirst(t *testing.T) {
	pathDir := t.TempDir()
	writeFakePHP(t, pathDir+"/php8.1", "8.1.2")
	writeFakePHP(t, pathDir+"/php8.4", "8.4.1")
	writeFakePHP(t, pathDir+"/php8.3", "8.3.7")

	t.Setenv("PATH", pathDir)
	t.Setenv("PHP_BINARY", pathDir+"/php8.1")

	installations := DiscoverPHPInstallations(t.Context())

	// PHP_BINARY stays first even though it is older
	require.NotEmpty(t, installations)
	assert.Equal(t, PHPSourceEnv, installations[0].Source)
	assert.Equal(t, canonicalPath(t, pathDir+"/php8.1"), installations[0].Binary)

	// Among our PATH fakes, newer versions come first after PHP_BINARY.
	php84 := findInstallation(installations, canonicalPath(t, pathDir+"/php8.4"))
	php83 := findInstallation(installations, canonicalPath(t, pathDir+"/php8.3"))
	require.NotNil(t, php84)
	require.NotNil(t, php83)

	idx84, idx83 := -1, -1
	for i, installation := range installations {
		switch installation.Binary {
		case php84.Binary:
			idx84 = i
		case php83.Binary:
			idx83 = i
		}
	}
	assert.Less(t, idx84, idx83)
}

// Debian-packaged PHP reports versions such as "8.3.6-0ubuntu0.24.04.1".
// phpdiscover keeps that suffix on purpose (it is what PHP prints, and its own
// comparisons ignore it), but github.com/shyim/go-version cannot parse it at all,
// so newPHPInstallation must strip it or those installations would be silently
// excluded from every constraint check. Do not "simplify" that into
// phpdiscover's Version.String().
func TestDiscoverPHPInstallationsNormalizesDistroVersionSuffix(t *testing.T) {
	pathDir := t.TempDir()
	writeFakePHP(t, pathDir+"/php", "8.3.6-0ubuntu0.24.04.1")

	t.Setenv("PATH", pathDir)
	t.Setenv("PHP_BINARY", "")

	installations := DiscoverPHPInstallations(t.Context())

	installation := findInstallation(installations, canonicalPath(t, pathDir+"/php"))
	require.NotNil(t, installation)
	assert.Equal(t, "8.3.6", installation.Version)

	// The normalized version must still satisfy a constraint on that series,
	// which is what the suffix would break.
	assert.NotEmpty(t, FilterCompatiblePHP(installations, versionSetChecker{"8.3.6": true}))
}

func TestFilterCompatiblePHP(t *testing.T) {
	installations := []PHPInstallation{
		{Binary: "/usr/bin/php8.4", Version: "8.4.1"},
		{Binary: "/usr/bin/php8.3", Version: "8.3.7"},
		{Binary: "/usr/bin/php8.1", Version: "8.1.2"},
	}

	compatible := FilterCompatiblePHP(installations, versionSetChecker{"8.3.7": true, "8.4.1": true})
	assert.Len(t, compatible, 2)
	assert.Equal(t, "8.4.1", compatible[0].Version)
	assert.Equal(t, "8.3.7", compatible[1].Version)

	all := FilterCompatiblePHP(installations, nil)
	assert.Len(t, all, 3)
}

func TestPreferredPHPInstallation(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		assert.Nil(t, PreferredPHPInstallation(nil))
	})

	t.Run("prefers PHP_BINARY", func(t *testing.T) {
		installations := []PHPInstallation{
			{Binary: "/env/php", Version: "8.2.0", Source: PHPSourceEnv},
			{Binary: "/usr/bin/php", Version: "8.4.0", Source: PHPSourcePath},
		}
		assert.Equal(t, "/env/php", PreferredPHPInstallation(installations).Binary)
	})

	t.Run("falls back to PATH default", func(t *testing.T) {
		installations := []PHPInstallation{
			{Binary: "/opt/homebrew/php", Version: "8.4.0", Source: phpdiscover.SourceHomebrew},
			{Binary: "/usr/bin/php8.4", Version: "8.4.0", Source: PHPSourcePath},
			{Binary: "/usr/bin/php", Version: "8.3.0", Source: PHPSourcePath, Default: true},
		}
		assert.Equal(t, "/usr/bin/php", PreferredPHPInstallation(installations).Binary)
	})

	t.Run("falls back to newest", func(t *testing.T) {
		installations := []PHPInstallation{
			{Binary: "/opt/homebrew/php8.4", Version: "8.4.0", Source: phpdiscover.SourceHomebrew},
			{Binary: "/opt/homebrew/php8.3", Version: "8.3.0", Source: phpdiscover.SourceHomebrew},
		}
		assert.Equal(t, "/opt/homebrew/php8.4", PreferredPHPInstallation(installations).Binary)
	})
}

func TestProbePHPBinary(t *testing.T) {
	dir := t.TempDir()
	writeFakePHP(t, dir+"/php", "8.3.7")

	t.Run("valid binary", func(t *testing.T) {
		installation, err := ProbePHPBinary(t.Context(), dir+"/php", PHPSourceFlag)
		assert.NoError(t, err)
		assert.Equal(t, canonicalPath(t, dir+"/php"), installation.Binary)
		assert.Equal(t, "8.3.7", installation.Version)
		assert.Equal(t, PHPSourceFlag, installation.Source)
	})

	t.Run("missing binary", func(t *testing.T) {
		_, err := ProbePHPBinary(t.Context(), dir+"/does-not-exist", PHPSourceFlag)
		assert.ErrorContains(t, err, "is not an executable file")
	})

	t.Run("not executable", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("executable bits are not meaningful on windows")
		}
		assert.NoError(t, os.WriteFile(dir+"/data.txt", []byte("hello"), 0o644))
		_, err := ProbePHPBinary(t.Context(), dir+"/data.txt", PHPSourceFlag)
		assert.ErrorContains(t, err, "is not an executable file")
	})

	t.Run("broken binary", func(t *testing.T) {
		assert.NoError(t, os.WriteFile(dir+"/broken", []byte("#!/bin/sh\nexit 3\n"), 0o755))
		_, err := ProbePHPBinary(t.Context(), dir+"/broken", PHPSourceFlag)
		assert.ErrorContains(t, err, "did not report a PHP version")
	})
}

func TestPHPInstallationString(t *testing.T) {
	installation := PHPInstallation{Binary: "/opt/homebrew/opt/php@8.3/bin/php", Version: "8.3.19", Source: phpdiscover.SourceHomebrew}
	assert.Equal(t, fmt.Sprintf("PHP %s — %s", "8.3.19", "/opt/homebrew/opt/php@8.3/bin/php"), installation.String())
}

func TestFindPHPByBinary(t *testing.T) {
	installations := []PHPInstallation{
		{Binary: "/php83", Version: "8.3.2"},
		{Binary: "/php82", Version: "8.2.9"},
	}

	t.Run("finds a known binary", func(t *testing.T) {
		found := FindPHPByBinary(installations, "/php82")
		assert.NotNil(t, found)
		assert.Equal(t, "8.2.9", found.Version)
	})

	t.Run("returns nil for an unknown binary", func(t *testing.T) {
		assert.Nil(t, FindPHPByBinary(installations, "/php81"))
	})

	t.Run("returns nil for an empty binary", func(t *testing.T) {
		assert.Nil(t, FindPHPByBinary(installations, ""))
	})
}

func TestFindPHPByVersionPin(t *testing.T) {
	installations := []PHPInstallation{
		{Binary: "/php8-30", Version: "8.30.1"},
		{Binary: "/php85", Version: "8.5.9"},
		{Binary: "/php83-new", Version: "8.3.33"},
		{Binary: "/php83-old", Version: "8.3.19"},
	}

	t.Run("a major.minor pin takes the newest patch release", func(t *testing.T) {
		found := FindPHPByVersionPin(installations, "8.3")
		require.NotNil(t, found)
		assert.Equal(t, "/php83-new", found.Binary)
	})

	t.Run("a full version pins that exact patch release", func(t *testing.T) {
		found := FindPHPByVersionPin(installations, "8.3.19")
		require.NotNil(t, found)
		assert.Equal(t, "/php83-old", found.Binary)
	})

	t.Run("a major-only pin takes the newest of that major", func(t *testing.T) {
		found := FindPHPByVersionPin(installations, "8")
		require.NotNil(t, found)
		assert.Equal(t, "/php8-30", found.Binary)
	})

	t.Run("components are compared whole, so 8.3 does not match 8.30", func(t *testing.T) {
		assert.Nil(t, FindPHPByVersionPin([]PHPInstallation{{Binary: "/php8-30", Version: "8.30.1"}}, "8.3"))
	})

	t.Run("an empty pin matches nothing", func(t *testing.T) {
		assert.Nil(t, FindPHPByVersionPin(installations, ""))
	})
}
