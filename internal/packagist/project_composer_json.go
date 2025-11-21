package packagist

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"text/template"

	"github.com/shopware/shopware-cli/logging"
)

func GenerateComposerJson(ctx context.Context, version string, rc bool, useDocker bool, withoutElasticsearch bool, noAudit bool) (string, error) {
	tplContent, err := template.New("composer.json").Parse(`{
    "name": "shopware/production",
    "license": "MIT",
    "type": "project",
    "require": {
        "composer-runtime-api": "^2.0",
        "shopware/administration": "{{ .DependingVersions }}",
        "shopware/core": "{{ .Version }}",
		{{if .UseElasticsearch}}
        "shopware/elasticsearch": "{{ .DependingVersions }}",
		{{end}}
        "shopware/storefront": "{{ .DependingVersions }}",
		{{if .UseDocker}}
		"shopware/docker-dev": "*",
		{{end}}
        "symfony/flex": "~2"
    },
    "repositories": [
        {
            "type": "path",
            "url": "custom/plugins/*",
            "options": {
                "symlink": true
            }
        },
        {
            "type": "path",
            "url": "custom/plugins/*/packages/*",
            "options": {
                "symlink": true
            }
        },
        {
            "type": "path",
            "url": "custom/static-plugins/*",
            "options": {
                "symlink": true
            }
        }
    ],
	"autoload": {
        "psr-4": {
            "App\\": "src/"
        }
    },
	{{if .RC}}
    "minimum-stability": "RC",
	{{end}}
    "prefer-stable": true,
    "config": {
        "allow-plugins": {
            "symfony/flex": true,
            "symfony/runtime": true
        },
        "optimize-autoloader": true,
        "sort-packages": true{{if .NoAudit}},
        "audit": {
            "block-insecure": false
        }
        {{end}}
    },
    "scripts": {
        "auto-scripts": [
        ],
        "post-install-cmd": [
            "@auto-scripts"
        ],
        "post-update-cmd": [
            "@auto-scripts"
        ]
    },
    "extra": {
        "symfony": {
            "allow-contrib": true,
            "endpoint": [
                "https://raw.githubusercontent.com/shopware/recipes/flex/main/index.json",
                "flex://defaults"
            ]
        }
    }
}`)
	if err != nil {
		return "", err
	}

	buf := new(bytes.Buffer)

	dependingVersions := "*"

	if strings.HasPrefix(version, "dev-") {
		fallbackVersion, err := getLatestFallbackVersion(ctx, strings.TrimPrefix(version, "dev-"))
		if err != nil {
			return "", err
		}

		if strings.HasPrefix(version, "dev-6") {
			version = strings.TrimPrefix(version, "dev-") + "-dev"
		}

		version = fmt.Sprintf("%s as %s.9999999-dev", version, fallbackVersion)
		dependingVersions = version
	}

	err = tplContent.Execute(buf, map[string]interface{}{
		"Version":           version,
		"DependingVersions": dependingVersions,
		"RC":                rc,
		"UseDocker":         useDocker,
		"UseElasticsearch":  !withoutElasticsearch,
		"NoAudit":           noAudit,
	})
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

var kernelFallbackRegExp = regexp.MustCompile(`(?m)SHOPWARE_FALLBACK_VERSION\s*=\s*'?"?(\d+\.\d+)`)

func getLatestFallbackVersion(ctx context.Context, branch string) (string, error) {
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://raw.githubusercontent.com/shopware/core/refs/heads/%s/Kernel.php", branch), http.NoBody)
	if err != nil {
		return "", err
	}

	r.Header.Set("User-Agent", "shopware-cli")

	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("could not fetch kernel.php from branch %s", branch)
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			logging.FromContext(context.Background()).Errorf("getLatestFallbackVersion: %v", err)
		}
	}()

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	matches := kernelFallbackRegExp.FindSubmatch(content)

	if len(matches) < 2 {
		return "", fmt.Errorf("could not determine shopware version")
	}

	return string(matches[1]), nil
}
