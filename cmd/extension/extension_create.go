package extension

import (
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/spf13/cobra"
)

func newCreateCmd() *cobra.Command {
	// CreateOptions holds the options for creating a new extension.
	opts := &extension.CreateOptions{}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new extension",
		Long:  "Create a new extension with the specified parameters.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return extension.Create(*opts)
		},
	}

	// Define flags for the command
	fs := cmd.Flags()
	fs.StringVar(&opts.Name, "name", "", "Name of the extension")
	fs.StringVarP(&opts.Namespace, "namespace", "s", "", "Namespace of the extension")
	fs.BoolVarP(&opts.AllExamples, "all-examples", "a", false, "Include all examples")
	fs.BoolVar(&opts.ConsoleCommand, "console-command", false, "Include a console command")
	fs.BoolVar(&opts.ScheduledTask, "scheduled-task", false, "Include a scheduled task")
	fs.BoolVar(&opts.EventSubscriber, "event-subscriber", false, "Include an event subscriber")
	fs.BoolVar(&opts.Controller, "controller", false, "Include a controller")
	fs.BoolVar(&opts.Route, "route", false, "Include a route")
	fs.StringVar(&opts.Entities, "entities", "", "Include entities")
	fs.BoolVar(&opts.JavascriptPlugin, "javascript-plugin", false, "Include a JavaScript plugin")
	fs.BoolVar(&opts.AdminModule, "admin-module", false, "Include an admin module")
	fs.BoolVar(&opts.CustomFieldset, "custom-fieldset", false, "Include a custom fieldset")

	return cmd
}

func init() {
	extensionRootCmd.AddCommand(newCreateCmd())
}
