package account

import (
	"fmt"

	"github.com/spf13/cobra"

	accountApi "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/logging"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login into your Shopware Account",
	Long:  "",
	RunE: func(cmd *cobra.Command, _ []string) error {
		client, err := accountApi.NewApi(cmd.Context(), nil)
		if err != nil {
			return err
		}

		fmt.Println(client.Token)

		logging.FromContext(cmd.Context()).Infof("Loggedin as %s", client.Token.Extra("email"))

		return nil
	},
}

func init() {
	accountRootCmd.AddCommand(loginCmd)
}
