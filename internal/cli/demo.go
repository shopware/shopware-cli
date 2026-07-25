package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

// NewDemoLoginCmd builds the demo `demo login` command. It is a worked example
// of the recommended architecture: explicit construction (no init()), deps drawn
// from the lazy Services container, a structured result type, and rendering via
// the presenter in context — never a direct fmt.Print. It proves all four
// patterns compose in app_test.go.
func NewDemoLoginCmd(svc Services) *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Log in to the Shopware Account (demo)",
		RunE: RunHandler(func(cmd *cobra.Command, _ []string) error {
			client, err := svc.AccountClient(cmd.Context())
			if err != nil {
				return fmt.Errorf("demo login: %w", err)
			}

			res, err := client.Login(cmd.Context())
			if err != nil {
				return err
			}

			// One structured render through the presenter; --output decides the
			// shape. No fmt.Println here.
			FromContext(cmd.Context()).Result(res)
			return nil
		}),
	}
}

// NewDemoRootCmd returns the `demo` parent the login command hangs off. It is
// the explicit-tree analogue of cmd/account/account.go.
func NewDemoRootCmd(svc Services) *cobra.Command {
	root := &cobra.Command{Use: "demo", Short: "Demo of the recommended Go architecture"}
	root.AddCommand(NewDemoLoginCmd(svc))
	return root
}

// fakeAccountClient is the test double for AccountClient.
type fakeAccountClient struct {
	user  string
	login func(ctx context.Context) (LoginResult, error)
}

func (f fakeAccountClient) Login(ctx context.Context) (LoginResult, error) {
	if f.login != nil {
		return f.login(ctx)
	}
	return LoginResult{User: f.user}, nil
}

// fakeServices is a Services double for app_test.go. It counts AccountClient
// calls so the test asserts laziness (built once even if asked twice).
type fakeServices struct {
	calls int
	inner AccountClient
	err   error
}

func (s *fakeServices) AccountClient(context.Context) (AccountClient, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	if s.inner != nil {
		return s.inner, nil
	}
	return fakeAccountClient{user: "demo@example.com"}, nil
}

// recordingTelemetry records the last Finish call so the test can assert result
// kind and command name without a real tracking backend.
type recordingTelemetry struct {
	lastCmd    *CommandInfo
	lastResult *ResultKind
	lastDur    *int64
}

func (r *recordingTelemetry) Finish(_ context.Context, cmd CommandInfo, result ResultKind, dur int64) {
	r.lastCmd = &cmd
	r.lastResult = &result
	r.lastDur = &dur
}

