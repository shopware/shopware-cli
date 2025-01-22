package esbuild

import (
	"context"
	_ "embed"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"

	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/logging"
)

const dartSassVersion = "1.69.7"

//go:embed static/variables.scss
var scssVariables []byte

//go:embed static/mixins.scss
var scssMixins []byte

func locateDartSass(ctx context.Context) (string, error) {
	if exePath, err := exec.LookPath("dart-sass"); err == nil {
		return exePath, nil
	}

	cacheDir := path.Join(system.GetShopwareCliCacheDir(), "dart-sass", dartSassVersion)

	expectedPath := path.Join(cacheDir, "sass")

	//goland:noinspection ALL
	if runtime.GOOS == "windows" {
		expectedPath += ".bat"
	}

	if _, err := os.Stat(expectedPath); err == nil {
		return expectedPath, nil
	}

	if _, err := os.Stat(filepath.Dir(expectedPath)); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(expectedPath), os.ModePerm); err != nil {
			return "", err
		}
	}

	logging.FromContext(ctx).Infof("Downloading dart-sass")

	if err := downloadDartSass(ctx, cacheDir); err != nil {
		return "", err
	}

	return expectedPath, nil
}
