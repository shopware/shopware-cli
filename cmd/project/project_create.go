package project

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/shyim/go-version"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/packagist"
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

		useDocker, _ := cmd.PersistentFlags().GetBool("docker")
		withoutElasticsearch, _ := cmd.PersistentFlags().GetBool("without-elasticsearch")
		noAudit, _ := cmd.PersistentFlags().GetBool("no-audit")

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
			// pick the most recent stable (non-RC) version
			for _, v := range filteredVersions {
				vs := v.String()
				if !strings.Contains(strings.ToLower(vs), "rc") {
					chooseVersion = vs
					break
				}
			}
			// if no stable found, fall back to top entry
			if chooseVersion == "" && len(filteredVersions) > 0 {
				chooseVersion = filteredVersions[0].String()
			}
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

		composerJson, err := packagist.GenerateComposerJson(cmd.Context(), chooseVersion, strings.Contains(chooseVersion, "rc"), useDocker, withoutElasticsearch, noAudit)
		if err != nil {
			return err
		}

		if err := os.WriteFile(filepath.Join(projectFolder, "composer.json"), []byte(composerJson), os.ModePerm); err != nil {
			return err
		}

		if err := os.WriteFile(filepath.Join(projectFolder, ".env"), []byte(""), os.ModePerm); err != nil {
			return err
		}

		if err := os.WriteFile(filepath.Join(projectFolder, ".env.local"), []byte(""), os.ModePerm); err != nil {
			return err
		}

		if err := os.WriteFile(filepath.Join(projectFolder, ".gitignore"), []byte("/.idea\n/vendor"), os.ModePerm); err != nil {
			return err
		}

		if err := os.MkdirAll(filepath.Join(projectFolder, "custom", "plugins"), os.ModePerm); err != nil {
			return err
		}

		if err := os.MkdirAll(filepath.Join(projectFolder, "custom", "static-plugins"), os.ModePerm); err != nil {
			return err
		}

		if err := os.WriteFile(filepath.Join(projectFolder, "php.ini"), []byte("memory_limit=512M"), os.ModePerm); err != nil {
			return err
		}

		logging.FromContext(cmd.Context()).Infof("Installing dependencies")

		var cmdInstall *exec.Cmd

		if useDocker {
			// Use Docker to run composer
			absProjectFolder, err := filepath.Abs(projectFolder)
			if err != nil {
				return err
			}

			dockerArgs := []string{"run", "--rm",
				"-v", fmt.Sprintf("%s:/app", absProjectFolder),
				"-w", "/app",
				"ghcr.io/shopware/docker-dev:php8.3-node22-caddy",
				"composer", "install", "--no-interaction"}

			cmdInstall = exec.CommandContext(cmd.Context(), "docker", dockerArgs...)
			cmdInstall.Stdout = os.Stdout
			cmdInstall.Stderr = os.Stderr

			return cmdInstall.Run()
		} else {
			// Use local composer
			composerBinary, err := exec.LookPath("composer")
			if err != nil {
				return err
			}

			phpBinary := os.Getenv("PHP_BINARY")

			if phpBinary != "" {
				cmdInstall = exec.CommandContext(cmd.Context(), phpBinary, composerBinary, "install")
			} else {
				cmdInstall = exec.CommandContext(cmd.Context(), "composer", "install")
			}

			cmdInstall.Dir = projectFolder
			cmdInstall.Stdin = os.Stdin
			cmdInstall.Stdout = os.Stdout
			cmdInstall.Stderr = os.Stderr

			return cmdInstall.Run()
		}
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
	projectCreateCmd.PersistentFlags().Bool("docker", false, "Use Docker to run Composer instead of local installation")
	projectCreateCmd.PersistentFlags().Bool("without-elasticsearch", false, "Remove Elasticsearch from the installation")
	projectCreateCmd.PersistentFlags().Bool("no-audit", false, "Disable composer audit blocking insecure packages")
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
