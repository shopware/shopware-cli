package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/shopware/shopware-cli/internal/ai/recommend"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/logging"
)

// printAIHint writes a one-line recommendation to install the Shopware CLI skill
// when the CLI was invoked by a detected AI client. It is non-blocking (w is
// stderr) and stays silent in non-interactive, CI, `--no-ai-hint` or `ai` runs.
func printAIHint(ctx context.Context, w io.Writer, args []string) {
	if !system.IsInteractionEnabled(ctx) || os.Getenv("CI") != "" {
		return
	}
	if slices.Contains(args, "--no-ai-hint") || slices.Contains(args, "ai") {
		return
	}

	client, msg := recommend.Suggest()
	if msg == "" {
		return
	}

	_, _ = fmt.Fprintln(w, msg)
	if err := recommend.MarkShown(client); err != nil {
		logging.FromContext(ctx).Debugf("could not record ai recommendation: %v", err)
	}
}
