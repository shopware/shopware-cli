package packagist

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

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

	require := newOrderedMap()
	require.set("composer-runtime-api", "^2.0")
	require.set("shopware/administration", opts.DependingVersion)
	require.set("shopware/core", opts.Version)
	if opts.UseElasticsearch {
		require.set("shopware/elasticsearch", opts.DependingVersion)
	}
	require.set("shopware/storefront", opts.DependingVersion)
	if opts.UseDocker {
		require.set("shopware/docker-dev", "*")
	}
	if opts.IsDeployer() {
		require.set("deployer/deployer", "*")
	}
	if opts.IsPlatformSH() {
		require.set("shopware/paas-meta", "*")
	}
	if opts.IsShopwarePaaS() {
		require.set("shopware/k8s-meta", "*")
	}
	require.set("symfony/flex", "~2")

	allowPlugins := newOrderedMap()
	allowPlugins.set("symfony/flex", true)
	allowPlugins.set("symfony/runtime", true)

	config := newOrderedMap()
	config.set("allow-plugins", allowPlugins)
	config.set("optimize-autoloader", true)
	config.set("sort-packages", true)
	if opts.NoAudit {
		audit := newOrderedMap()
		audit.set("block-insecure", false)
		config.set("audit", audit)
	}
	if opts.IsShopwarePaaS() {
		platform := newOrderedMap()
		platform.set("ext-grpc", "1.44.0")
		platform.set("ext-opentelemetry", "3.21.0")
		config.set("platform", platform)
	}

	composer := newOrderedMap()
	composer.set("name", "shopware/production")
	composer.set("license", "MIT")
	composer.set("type", "project")
	composer.set("require", require)
	symlinkOptions := newOrderedMap()
	symlinkOptions.set("symlink", true)

	repo1 := newOrderedMap()
	repo1.set("type", "path")
	repo1.set("url", "custom/plugins/*")
	repo1.set("options", symlinkOptions)

	repo2 := newOrderedMap()
	repo2.set("type", "path")
	repo2.set("url", "custom/plugins/*/packages/*")
	repo2.set("options", symlinkOptions)

	repo3 := newOrderedMap()
	repo3.set("type", "path")
	repo3.set("url", "custom/static-plugins/*")
	repo3.set("options", symlinkOptions)

	composer.set("repositories", []*orderedMap{repo1, repo2, repo3})
	psr4 := newOrderedMap()
	psr4.set("App\\", "src/")
	autoload := newOrderedMap()
	autoload.set("psr-4", psr4)
	composer.set("autoload", autoload)
	if opts.RC {
		composer.set("minimum-stability", "RC")
	}
	composer.set("prefer-stable", true)
	composer.set("config", config)
	scripts := newOrderedMap()
	scripts.set("auto-scripts", []string{})
	scripts.set("post-install-cmd", []string{"@auto-scripts"})
	scripts.set("post-update-cmd", []string{"@auto-scripts"})
	composer.set("scripts", scripts)
	symfony := newOrderedMap()
	symfony.set("allow-contrib", true)
	symfony.set("endpoint", []string{
		"https://raw.githubusercontent.com/shopware/recipes/flex/main/index.json",
		"flex://defaults",
	})
	extra := newOrderedMap()
	extra.set("symfony", symfony)
	composer.set("extra", extra)

	result, err := json.MarshalIndent(composer, "", "    ")
	if err != nil {
		return "", err
	}

	return string(result) + "\n", nil
}

// orderedMap preserves insertion order for JSON marshaling.
type orderedMap struct {
	keys   []string
	values map[string]any
}

func newOrderedMap() *orderedMap {
	return &orderedMap{
		keys:   []string{},
		values: make(map[string]any),
	}
}

func (o *orderedMap) set(key string, value any) {
	if _, exists := o.values[key]; !exists {
		o.keys = append(o.keys, key)
	}
	o.values[key] = value
}

func (o *orderedMap) MarshalJSON() ([]byte, error) {
	var buf strings.Builder
	buf.WriteString("{")
	for i, key := range o.keys {
		if i > 0 {
			buf.WriteString(",")
		}
		keyJSON, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		valueJSON, err := json.Marshal(o.values[key])
		if err != nil {
			return nil, err
		}
		buf.Write(keyJSON)
		buf.WriteString(":")
		buf.Write(valueJSON)
	}
	buf.WriteString("}")
	return []byte(buf.String()), nil
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
