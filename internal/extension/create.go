package extension

type CreateOptions struct {
	Name             string
}

// Create reads the command line arguments and creates a new extension based on the provided parameters.
func Create(opts CreateOptions) error {
	// parse and validate options
	
	// invalid parameter = missing parameter

	// parameters missing? check if interactive mode is enabled. Else throw error.

	// start tui and ask for missing parameters, listen for input and validate

	// create the extension directory

	// create the files

	// inform user

	return nil
}
