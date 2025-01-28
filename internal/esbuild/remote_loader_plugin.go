package esbuild

import (
	"context"
	"crypto/md5"
	"fmt"
	"github.com/FriendsOfShopware/shopware-cli/internal/system"
	"github.com/evanw/esbuild/pkg/api"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
)

const RemoteLoaderName = "sw-remote-loader"

type RemoteLoaderOptions struct {
	BaseUrl  string
	Matchers map[string]RemoteLoaderReplacer
}

type RemoteLoaderReplacer struct {
	Matching *regexp.Regexp
	Replace  string
}

func newRemoteLoaderPlugin(ctx context.Context, options RemoteLoaderOptions) api.Plugin {
	return api.Plugin{
		Name: RemoteLoaderName,
		Setup: func(build api.PluginBuild) {
			for matcher, matcherOptions := range options.Matchers {
				build.OnResolve(api.OnResolveOptions{Filter: matcher}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
					file := options.BaseUrl + matcherOptions.Matching.ReplaceAllString(args.Path, matcherOptions.Replace)

					ext := filepath.Ext(file)

					// When we have a file extension, try direct load. But maybe the file has two file extensions, therefore, we need to fallback to .js/.ts
					if ext != "" {
						if content, err := fetchRemoteAsset(ctx, file); err == nil {
							return api.OnResolveResult{
								Path: content,
							}, nil
						}
					}

					// Try to load the file with .ts and .js extension
					if content, err := fetchRemoteAsset(ctx, file+".ts"); err == nil {
						return api.OnResolveResult{
							Path: content,
						}, nil
					}

					if content, err := fetchRemoteAsset(ctx, file+".js"); err == nil {
						return api.OnResolveResult{
							Path: content,
						}, nil
					}

					return api.OnResolveResult{}, fmt.Errorf("could not load file %s", file)
				})
			}

			build.OnResolve(api.OnResolveOptions{Filter: "deepmerge"}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
				if file, err := fetchRemoteAsset(ctx, "https://unpkg.com/deepmerge@4.3.1"); err == nil {
					return api.OnResolveResult{
						Path: file,
					}, nil
				}

				return api.OnResolveResult{}, fmt.Errorf("could not load file %s", args.Path)
			})
		},
	}
}

func fetchRemoteAsset(ctx context.Context, url string) (string, error) {
	assetDir := path.Join(system.GetShopwareCliCacheDir(), "assets")

	if _, err := os.Stat(assetDir); os.IsNotExist(err) {
		if err := os.MkdirAll(assetDir, 0755); err != nil {
			return "", err
		}
	}

	cacheFile := path.Join(assetDir, fmt.Sprintf("%x", md5.Sum([]byte(url))))

	ext := filepath.Ext(url)

	// Only add file extension for those files. Required because of HTTP requests to unpkg.com
	if ext == ".css" || ext == ".scss" {
		cacheFile += ext
	}

	cacheMissFile := cacheFile + ".miss"

	if _, err := os.Stat(cacheFile); err == nil {
		return cacheFile, nil
	}

	if _, err := os.Stat(cacheMissFile); err == nil {
		return "", fmt.Errorf("file does not exists")
	}

	r, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	r.Header.Add("User-Agent", "Shopware CLI")

	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		_ = os.WriteFile(cacheMissFile, []byte{}, 0644)

		return "", fmt.Errorf("file does not exists")
	}

	content, err := io.ReadAll(resp.Body)

	if err != nil {
		return "", err
	}

	if err := resp.Body.Close(); err != nil {
		return "", err
	}

	if err := os.WriteFile(cacheFile, content, 0644); err != nil {
		return "", err
	}

	return cacheFile, nil
}
