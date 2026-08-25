package project

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/curl"
)

var skipDefaultHeaders bool

var projectAdminApiCmd = &cobra.Command{
	Use:   "admin-api [method] [path]",
	Short: "Pre authenticated curl interface to the Admin API",
	RunE: func(cobraCmd *cobra.Command, args []string) error {
		projectRoot, err := findClosestShopwareProject(true)
		if err != nil {
			return err
		}

		cmdExecutor, err := resolveExecutor(cobraCmd, projectRoot)
		if err != nil {
			return err
		}

		cfg := cmdExecutor.ShopConfig()
		if cfg == nil || cfg.AdminApi == nil {
			return errors.New("admin api is not activated in the config")
		}

		client, err := cmdExecutor.AdminAPIClient(cobraCmd.Context())
		if err != nil {
			return err
		}

		token, err := client.Token().Token()
		if err != nil {
			return err
		}

		tokenOnly, _ := cobraCmd.PersistentFlags().GetBool("output-token")

		if tokenOnly {
			fmt.Println(token)
			return nil
		}

		if len(args) < 2 {
			return errors.New("command needs 2 arguments")
		}

		shopURL, err := url.Parse(cfg.URL)
		if err != nil {
			return err
		}

		apiPath, err := parsePath(args[1])
		if err != nil {
			return err
		}

		fullURL := shopURL.ResolveReference(apiPath)

		commandConfig := []curl.Config{
			curl.Url(fullURL),
			curl.Method(args[0]),
			curl.BearerToken(token.AccessToken),
			curl.Args(args[2:]),
		}

		if cfg.AdminApi.DisableSSLCheck {
			commandConfig = append(commandConfig, curl.Args([]string{"--insecure"}))
		}

		if !skipDefaultHeaders {
			commandConfig = append(commandConfig, curl.Header("content-type", "application/json"))
			commandConfig = append(commandConfig, curl.Header("accept", "application/json"))
		}

		cmd := curl.InitCurlCommand(commandConfig...)

		return cmd.Run(cobraCmd.Context())
	},
}

func parsePath(inputPath string) (*url.URL, error) {
	inputPath = strings.TrimPrefix(inputPath, "/api")
	inputPath = strings.TrimPrefix(inputPath, "api")
	return url.Parse(path.Join("api", inputPath))
}

func init() {
	projectAdminApiCmd.PersistentFlags().Bool("output-token", false, "Output only token")
	projectAdminApiCmd.PersistentFlags().BoolVarP(
		&skipDefaultHeaders,
		"no-default-headers",
		"",
		false,
		"skips setting the content-type and accept headers",
	)
	projectRootCmd.AddCommand(projectAdminApiCmd)
}
