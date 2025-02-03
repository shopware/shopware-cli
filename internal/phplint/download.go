package phplint

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"

	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/logging"
)

func findPHPWasmFile(ctx context.Context, phpVersion string) ([]byte, error) {
	expectedFile := "php-" + phpVersion + ".wasm"
	expectedPathLocation := path.Join(system.GetShopwareCliCacheDir(), "wasm", "php", expectedFile)

	if _, err := os.Stat(expectedPathLocation); err == nil {
		return os.ReadFile(expectedPathLocation)
	}

	logging.FromContext(ctx).Infof("Downloading PHP %s wasm build", phpVersion)

	downloadUrl := "https://github.com/shopwareLabs/php-cli-wasm-binaries/releases/download/1.0.0/" + expectedFile

	r, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadUrl, nil)
	if err != nil {
		return nil, err
	}

	r.Header.Set("accept", "application/octet-stream")

	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("cannot download php-wasm binary: %s (%s)", resp.Status, downloadUrl)
	}

	if _, err := os.Stat(path.Dir(expectedPathLocation)); os.IsNotExist(err) {
		if err := os.MkdirAll(path.Dir(expectedPathLocation), os.ModePerm); err != nil {
			return nil, fmt.Errorf("cannot create directory %s: %v", path.Dir(expectedPathLocation), err)
		}
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		_ = resp.Body.Close()

		return nil, fmt.Errorf("findPHPWasmFile: %v", err)
	}

	_ = resp.Body.Close()

	if err := os.WriteFile(expectedPathLocation, data, os.ModePerm); err != nil {
		logging.FromContext(ctx).Debugf("cannot write php-wasm binary to %s: %v", expectedPathLocation, err)
	}

	return data, nil
}
