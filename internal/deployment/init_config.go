package deployment

import (
	"encoding/json"
	"errors"
	"io"
)

// ReadInitializationConfig reads exactly one JSON object, rejecting misspelled fields.
// Decode errors deliberately omit input values because they may contain secrets.
func ReadInitializationConfig(input io.Reader) (*InitializationConfig, error) {
	decoder := json.NewDecoder(input)
	decoder.DisallowUnknownFields()
	var config *InitializationConfig
	if err := decoder.Decode(&config); err != nil || config == nil {
		return nil, errors.New("cannot read initialization config: expected a JSON object with runtime_values and optional install_values")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.New("cannot read initialization config: expected a single JSON object")
	}
	return config, nil
}
