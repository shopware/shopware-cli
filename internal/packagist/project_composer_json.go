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

const (
	DeploymentNone         = "none"
	DeploymentDeployer     = "deployer"
	DeploymentPlatformSH   = "platformsh"
	DeploymentShopwarePaaS = "shopware-paas"
)

type ComposerJsonOptions struct {
	Version          string
	DependingVersion string
	RC               bool
	UseDocker        bool
	UseMinio         bool
	UseElasticsearch bool
	NoAudit          bool
	DeploymentMethod string
}

func (o ComposerJsonOptions) IsShopwarePaaS() bool {
	return o.DeploymentMethod == DeploymentShopwarePaaS
}

func (o ComposerJsonOptions) IsPlatformSH() bool {
	return o.DeploymentMethod == DeploymentPlatformSH
}

func (o ComposerJsonOptions) IsDeployer() bool {
	return o.DeploymentMethod == DeploymentDeployer
}

func GenerateComposerJson(ctx context.Context, opts ComposerJsonOptions) (string, error) {
	tplContent, err := template.New("composer.json").Parse(`{
    "name": "shopware/production",
    "license": "MIT",
    "type": "project",
    "require": {
        "composer-runtime-api": "^2.0",
        "shopware/administration": "{{ .DependingVersion }}",
        "shopware/core": "{{ .Version }}",
		{{if .UseElasticsearch}}
        "shopware/elasticsearch": "{{ .DependingVersion }}",
		{{end}}
        "shopware/storefront": "{{ .DependingVersion }}",
		{{if .UseDocker}}
		"shopware/docker-dev": "*",
		{{end}}
		{{if .IsDeployer}}
		"deployer/deployer": "*",
		{{end}}
		{{if .IsPlatformSH}}
		"shopware/paas-meta": "*",
		{{end}}
		{{if .IsShopwarePaaS}}
		"shopware/k8s-meta": "*",
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
        }{{end}}{{if .IsShopwarePaaS}},
        "platform": {
            "ext-grpc": "1.44.0",
            "ext-opentelemetry": "3.21.0"
        }{{end}}
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

	opts.DependingVersion = "*"

	if strings.HasPrefix(opts.Version, "dev-") {
		fallbackVersion, err := getLatestFallbackVersion(ctx, strings.TrimPrefix(opts.Version, "dev-"))
		if err != nil {
			return "", err
		}

		if strings.HasPrefix(opts.Version, "dev-6") {
			opts.Version = strings.TrimPrefix(opts.Version, "dev-") + "-dev"
		}

		opts.Version = fmt.Sprintf("%s as %s.9999999-dev", opts.Version, fallbackVersion)
		opts.DependingVersion = opts.Version
	}

	err = tplContent.Execute(buf, opts)
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
