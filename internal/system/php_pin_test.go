package system

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPHPVersionPin(t *testing.T) {
	// The patch level is dropped: pinning it would break on every PHP update.
	assert.Equal(t, "8.3", PHPVersionPin("8.3.19"))
	assert.Equal(t, "8.3", PHPVersionPin("8.3"))
	assert.Equal(t, "8.4", PHPVersionPin("8.4.24"))
	assert.Equal(t, "", PHPVersionPin(""))
}

func TestResolvePinnedPHPBinaryMatching(t *testing.T) {
	// Names must match what phpdiscover accepts: php, php8, php8.3, php8.3.19.
	pathDir := t.TempDir()
	writeFakePHP(t, pathDir+"/php", "8.5.9")
	writeFakePHP(t, pathDir+"/php8.3.19", "8.3.19")
	writeFakePHP(t, pathDir+"/php8.3", "8.3.33")

	t.Setenv("PATH", pathDir)
	t.Setenv("PHP_BINARY", "")

	t.Run("any patch release of the pinned series matches, newest wins", func(t *testing.T) {
		binary, err := ResolvePinnedPHPBinary(t.Context(), "8.3")
		assert.NoError(t, err)
		assert.Equal(t, canonicalPath(t, pathDir+"/php8.3"), binary)
	})

	t.Run("a full version pins that exact patch release", func(t *testing.T) {
		binary, err := ResolvePinnedPHPBinary(t.Context(), "8.3.19")
		assert.NoError(t, err)
		assert.Equal(t, canonicalPath(t, pathDir+"/php8.3.19"), binary)
	})

	t.Run("a major-only pin matches the newest of that major", func(t *testing.T) {
		binary, err := ResolvePinnedPHPBinary(t.Context(), "8")
		assert.NoError(t, err)
		assert.Equal(t, canonicalPath(t, pathDir+"/php"), binary)
	})

	t.Run("a missing series reports what was discovered", func(t *testing.T) {
		_, err := ResolvePinnedPHPBinary(t.Context(), "8.2")

		var notFound *PHPVersionNotFoundError
		require.ErrorAs(t, err, &notFound)
		assert.Equal(t, "8.2", notFound.Pin)
		assert.NotEmpty(t, notFound.Installations)
		// Newest first, so the error lists the most relevant candidate up front.
		assert.Equal(t, "8.5.9", notFound.Installations[0].Version)
	})
}

func TestResolvePinnedPHPBinary(t *testing.T) {
	t.Run("an empty pin defers to the caller's fallback", func(t *testing.T) {
		binary, err := ResolvePinnedPHPBinary(t.Context(), "")
		assert.NoError(t, err)
		assert.Empty(t, binary)
	})

	t.Run("ignores PHP_BINARY", func(t *testing.T) {
		t.Setenv("PHP_BINARY", "/env/php")

		binary, err := ResolvePinnedPHPBinary(t.Context(), "")
		assert.NoError(t, err)
		assert.Empty(t, binary)
	})
}

func TestResolveProjectPHPBinary(t *testing.T) {
	t.Run("PHP_BINARY wins over the pin", func(t *testing.T) {
		dir := t.TempDir()
		writeFakePHP(t, dir+"/php", "8.2.9")
		t.Setenv("PHP_BINARY", dir+"/php")

		// An unsatisfiable pin proves the env var short-circuits the pin lookup
		// rather than merely being preferred among matches.
		binary, err := ResolveProjectPHPBinary(t.Context(), "5.6")
		assert.NoError(t, err)
		assert.Equal(t, canonicalPath(t, dir+"/php"), binary)
	})

	t.Run("an unusable PHP_BINARY fails instead of falling back", func(t *testing.T) {
		dir := t.TempDir()
		writeFakePHP(t, dir+"/php8.3", "8.3.19")
		t.Setenv("PATH", dir)
		t.Setenv("PHP_BINARY", "/does/not/exist/php")

		_, err := ResolveProjectPHPBinary(t.Context(), "8.3")
		assert.ErrorContains(t, err, "PHP_BINARY is set but unusable")

		var binErr *PHPBinaryError
		assert.ErrorAs(t, err, &binErr)
	})

	t.Run("a usable PHP_BINARY is returned canonicalized", func(t *testing.T) {
		dir := t.TempDir()
		writeFakePHP(t, dir+"/php", "8.3.19")
		t.Setenv("PHP_BINARY", dir+"/php")

		binary, err := ResolveProjectPHPBinary(t.Context(), "8.4")
		assert.NoError(t, err)
		assert.Equal(t, canonicalPath(t, dir+"/php"), binary)
	})

	t.Run("without PHP_BINARY an empty pin defers to the caller's fallback", func(t *testing.T) {
		t.Setenv("PHP_BINARY", "")

		binary, err := ResolveProjectPHPBinary(t.Context(), "")
		assert.NoError(t, err)
		assert.Empty(t, binary)
	})

	t.Run("without PHP_BINARY an unsatisfiable pin fails", func(t *testing.T) {
		t.Setenv("PHP_BINARY", "")

		_, err := ResolveProjectPHPBinary(t.Context(), "5.6")

		var notFound *PHPVersionNotFoundError
		assert.ErrorAs(t, err, &notFound)
		assert.Equal(t, "5.6", notFound.Pin)
	})
}

func TestPHPVersionNotFoundError(t *testing.T) {
	err := &PHPVersionNotFoundError{
		Pin: "8.3",
		Installations: []PHPInstallation{
			{Binary: "/php85", Version: "8.5.9", Source: PHPSourcePath},
		},
	}

	message := err.Error()
	assert.Contains(t, message, "PHP 8.3")
	assert.Contains(t, message, "php_version")
	// The discovered versions are listed so the user can see what exists.
	assert.Contains(t, message, "8.5.9")
}

// A pin can come from a PHP_BINARY-only install the library does not scan;
// resolving must consult the same set that wrote it.
func TestResolvePinnedPHPBinaryFindsPHPBinaryOnlyInstall(t *testing.T) {
	custom := t.TempDir()
	writeFakePHP(t, custom+"/php", "8.3.19")

	// Nothing on PATH, so the library alone would find nothing.
	t.Setenv("PATH", t.TempDir())
	t.Setenv("PHP_BINARY", custom+"/php")

	binary, err := ResolvePinnedPHPBinary(t.Context(), "8.3")
	assert.NoError(t, err)
	assert.Equal(t, canonicalPath(t, custom+"/php"), binary)
}

func TestProbePHPBinaryAcceptsBareCommandName(t *testing.T) {
	dir := t.TempDir()
	writeFakePHP(t, dir+"/php8.2", "8.2.9")
	t.Setenv("PATH", dir)

	installation, err := ProbePHPBinary(t.Context(), "php8.2", PHPSourceEnv)
	require.NoError(t, err)
	assert.Equal(t, "8.2.9", installation.Version)
	assert.Equal(t, canonicalPath(t, dir+"/php8.2"), installation.Binary)
}

func TestUnusablePHPBinaryEnv(t *testing.T) {
	t.Run("unset is fine", func(t *testing.T) {
		t.Setenv("PHP_BINARY", "")
		assert.NoError(t, UnusablePHPBinaryEnv(t.Context()))
	})

	t.Run("a usable binary is fine", func(t *testing.T) {
		dir := t.TempDir()
		writeFakePHP(t, dir+"/php", "8.3.19")
		t.Setenv("PHP_BINARY", dir+"/php")
		assert.NoError(t, UnusablePHPBinaryEnv(t.Context()))
	})

	t.Run("a missing binary is reported", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		t.Setenv("PHP_BINARY", "/does/not/exist/php")
		assert.Error(t, UnusablePHPBinaryEnv(t.Context()))
	})
}
