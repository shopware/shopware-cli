package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/shopware/shopware-cli/internal/ai/recommend"
	"github.com/shopware/shopware-cli/logging"
)

// suggest is a seam over recommend.Suggest so tests can stub it.
var suggest = recommend.Suggest

// printAIHint writes a one-line recommendation to install the Shopware CLI skill
// when the CLI was invoked by a detected AI client (AI_AGENT). It is non-blocking
// (w is stderr) and stays silent in CI, when `--no-ai-hint` is passed, while
// using `ai` commands, and when no agent is detected or the skill is already
// installed for it. The detection itself is the trigger, so it must not depend on
// stdin being a terminal — agents invoke the CLI non-interactively.
func printAIHint(ctx context.Context, w io.Writer, args []string) {
	if os.Getenv("CI") != "" || slices.Contains(args, "--no-ai-hint") || slices.Contains(args, "ai") {
		return
	}

	client, msg := suggest(ctx)
	if msg == "" {
		return
	}

	_, _ = fmt.Fprintln(w, msg)
	if err := recommend.MarkShown(client); err != nil {
		logging.FromContext(ctx).Debugf("could not record ai recommendation: %v", err)
	}
}
