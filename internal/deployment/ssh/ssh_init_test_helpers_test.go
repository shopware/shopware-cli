package ssh

import "context"

// Each host is inspected once before prompting and once during verification.
// The coordinator waits between phases, so each store is used sequentially.
type memoryInitializationStore struct {
	states      [2]sshDeploymentInitState
	inspectErrs [2]error
	inspections int
	writes      []sshDeploymentInitConfig
	applyErr    error
}

// Inspect returns the next scripted inspection result.
func (s *memoryInitializationStore) Inspect(context.Context) (sshDeploymentInitState, error) {
	i := s.inspections
	s.inspections++
	return s.states[i], s.inspectErrs[i]
}

// Apply records a configuration write and returns the scripted error.
func (s *memoryInitializationStore) Apply(_ context.Context, config sshDeploymentInitConfig) error {
	s.writes = append(s.writes, config)
	return s.applyErr
}
