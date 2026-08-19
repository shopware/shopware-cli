package extension

import (
	tea "charm.land/bubbletea/v2"
)

type CreateOptions struct {
	Name             string
	Namespace        string
	AllExamples      bool
	ConsoleCommand   bool
	ScheduledTask    bool
	EventSubscriber  bool
	Controller       bool
	Route            bool
	Entities         string
	JavascriptPlugin bool
	AdminModule      bool
	CustomFieldset   bool
}

// Create reads the command line arguments and creates a new extension based on the provided parameters.
func Create(opts CreateOptions) error {
	// parse and validate options
	// invalid parameter = missing parameter

	// parameters missing? check if interactive mode is enabled. Else throw error.

	// start tui and ask for missing parameters, listen for input and validate
	if err := startCreationInterface(&opts); err != nil {
		return err
	}

	// create the extension directory

	// create the files

	// inform user

	return nil
}

func startCreationInterface(opts *CreateOptions) error {
	// text input for name and namespace
	// description of the extension??
	// checkboxes for scaffolding options
	// each checkbox has a description and a help button
	form := newCreateForm(*opts)
	// this is not even my final form lol
	final, err := tea.NewProgram(form).Run()
	if err != nil {
		return err
	}

	result := final.(*createForm)
	if result.cancelled {
		return ErrCreationCancelled
	}
	result.values(opts)

	return nil
}
