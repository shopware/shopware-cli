package project

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"github.com/charmbracelet/huh"
	"github.com/shyim/go-version"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/logging"
)

var projectCreateCmd = &cobra.Command{
	Use:   "create [name] [version]",
	Short: "Create a new Shopware 6 project",
	Args:  cobra.MinimumNArgs(1),
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 1 {
			filteredVersions, err := getFilteredInstallVersions(cmd.Context())
			if err != nil {
				return []string{}, cobra.ShellCompDirectiveNoFileComp
			}
			versions := make([]string, 0)

			for i, v := range filteredVersions {
				versions[i] = v.String()
			}

			versions = append(versions, "latest")

			return versions, cobra.ShellCompDirectiveNoFileComp
		}

		return []string{}, cobra.ShellCompDirectiveFilterDirs
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		projectFolder := args[0]

		if _, err := os.Stat(projectFolder); err == nil {
			return fmt.Errorf("the folder %s exists already", projectFolder)
		}

		logging.FromContext(cmd.Context()).Infof("Using Symfony Flex to create a new Shopware 6 project")

		filteredVersions, err := getFilteredInstallVersions(cmd.Context())
		if err != nil {
			return err
		}

		var result string

		if len(args) == 2 {
			result = args[1]
		} else {
			options := make([]huh.Option[string], 0)
			for _, v := range filteredVersions {
				versionStr := v.String()
				options = append(options, huh.NewOption(versionStr, versionStr))
			}

			// Add "latest" option
			options = append(options, huh.NewOption("latest", "latest"))

			// Create and run the select form
			form := huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Height(10).
						Title("Select Version").
						Options(options...).
						Value(&result),
				),
			)

			if err := form.Run(); err != nil {
				return err
			}
		}

		chooseVersion := ""

		if result == "latest" {
			chooseVersion = filteredVersions[0].String()
		} else if strings.HasPrefix(result, "dev-") {
			chooseVersion = result
		} else {
			for _, release := range filteredVersions {
				if release.String() == result {
					chooseVersion = release.String()
					break
				}
			}
		}

		if chooseVersion == "" {
			return fmt.Errorf("cannot find version %s", result)
		}

		if err := os.Mkdir(projectFolder, os.ModePerm); err != nil {
			return err
		}

		logging.FromContext(cmd.Context()).Infof("Setting up Shopware %s", chooseVersion)

		composerJson, err := generateComposerJson(cmd.Context(), chooseVersion, strings.Contains(chooseVersion, "rc"))
		if err != nil {
			return err
		}

		if err := os.WriteFile(fmt.Sprintf("%s/composer.json", projectFolder), []byte(composerJson), os.ModePerm); err != nil {
			return err
		}

		if err := os.WriteFile(fmt.Sprintf("%s/.env", projectFolder), []byte(""), os.ModePerm); err != nil {
			return err
		}

		if err := os.WriteFile(fmt.Sprintf("%s/.env.local", projectFolder), []byte(""), os.ModePerm); err != nil {
			return err
		}

		if err := os.WriteFile(fmt.Sprintf("%s/.gitignore", projectFolder), []byte("/.idea\n/vendor"), os.ModePerm); err != nil {
			return err
		}

		if err := os.MkdirAll(fmt.Sprintf("%s/custom/plugins", projectFolder), os.ModePerm); err != nil {
			return err
		}

		if err := os.MkdirAll(fmt.Sprintf("%s/custom/static-plugins", projectFolder), os.ModePerm); err != nil {
			return err
		}

		if err := os.WriteFile(path.Join(projectFolder, "php.ini"), []byte("memory_limit=512M"), os.ModePerm); err != nil {
			return err
		}

		logging.FromContext(cmd.Context()).Infof("Installing dependencies")

		composerBinary, err := exec.LookPath("composer")
		if err != nil {
			return err
		}

		var cmdInstall *exec.Cmd
		phpBinary := os.Getenv("PHP_BINARY")

		if phpBinary != "" {
			cmdInstall = exec.Command(phpBinary, composerBinary, "install")
		} else {
			cmdInstall = exec.Command("composer", "install")
		}

		cmdInstall.Dir = projectFolder
		cmdInstall.Stdin = os.Stdin
		cmdInstall.Stdout = os.Stdout
		cmdInstall.Stderr = os.Stderr

		return cmdInstall.Run()
	},
}

func getFilteredInstallVersions(ctx context.Context) ([]*version.Version, error) {
	releases, err := fetchAvailableShopwareVersions(ctx)
	if err != nil {
		return nil, err
	}

	filteredVersions := make([]*version.Version, 0)
	constraint, _ := version.NewConstraint(">=6.4.18.0")

	for _, release := range releases {
		parsed := version.Must(version.NewVersion(release))

		if constraint.Check(parsed) {
			filteredVersions = append(filteredVersions, parsed)
		}
	}

	sort.Sort(sort.Reverse(version.Collection(filteredVersions)))

	return filteredVersions, nil
}

func init() {
	projectRootCmd.AddCommand(projectCreateCmd)
}

func fetchAvailableShopwareVersions(ctx context.Context) ([]string, error) {
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://releases.shopware.com/changelog/index.json", http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			logging.FromContext(ctx).Errorf("fetchAvailableShopwareVersions: %v", err)
		}
	}()

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var releases []string

	if err := json.Unmarshal(content, &releases); err != nil {
		return nil, err
	}

	return releases, nil
}

func generateComposerJson(ctx context.Context, version string, rc bool) (string, error) {
	tplContent, err := template.New("composer.json").Parse(`{
    "name": "shopware/production",
    "license": "MIT",
    "type": "project",
    "require": {
        "composer-runtime-api": "^2.0",
        "shopware/administration": "{{ .DependingVersions }}",
        "shopware/core": "{{ .Version }}",
        "shopware/elasticsearch": "{{ .DependingVersions }}",
        "shopware/storefront": "{{ .DependingVersions }}",
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
        "sort-packages": true
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
