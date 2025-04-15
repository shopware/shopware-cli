package extension

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCanIUseUpdate(t *testing.T) {
	tmpDir := t.TempDir()

	packageLock := `{
  "name": "test",
  "version": "1.0.0",
  "lockfileVersion": 3,
  "requires": true,
  "packages": {
    "": {
      "name": "test",
      "version": "1.0.0",
      "license": "ISC",
      "dependencies": {
        "caniuse-lite": "^1.0.30001570"
      }
    },
    "node_modules/caniuse-lite": {
      "version": "1.0.30001570",
      "resolved": "https://registry.npmjs.org/caniuse-lite/-/caniuse-lite-1.0.30001570.tgz",
      "integrity": "sha512-+3e0ASu4sw1SWaoCtvPeyXp+5PsjigkSt8OXZbF9StH5pQWbxEjLAZE3n8Aup5udop1uRiKA7a4utUk/uoSpUw==",
      "funding": [
        {
          "type": "opencollective",
          "url": "https://opencollective.com/browserslist"
        },
        {
          "type": "tidelift",
          "url": "https://tidelift.com/funding/github/npm/caniuse-lite"
        },
        {
          "type": "github",
          "url": "https://github.com/sponsors/ai"
        }
      ]
    }
  }
}`

	packageLockJson := path.Join(tmpDir, "package-lock.json")

	if err := os.WriteFile(packageLockJson, []byte(packageLock), 0644); err != nil {
		t.Fatal(err)
	}

	assert.NoError(t, patchPackageLockToRemoveCanIUsePackage(packageLockJson))

	updatedPackageLock, err := os.ReadFile(packageLockJson)

	assert.NoError(t, err)

	assert.NotContains(t, string(updatedPackageLock), "node_modules/caniuse-lite")
}
