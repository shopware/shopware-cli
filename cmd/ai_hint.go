package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/ai/agent"
	"github.com/shopware/shopware-cli/internal/ai/recommend"
	"github.com/shopware/shopware-cli/logging"
)

// suggest is a seam over recommend.Suggest so tests can stub it.
var suggest = recommend.Suggest

// printAIHint writes a short recommendation to install a Shopware skill when the
// CLI was invoked by a detected AI client (AI_AGENT). The skill comes from the
// invoked command's RecommendedSkillAnnotation (inherited from a parent). It is
// non-blocking (w is stderr) and stays silent in CI, during shell completion, when
// `--no-ai-hint` is passed, while running an `ai` command, and when no agent is
// detected or the skill is already installed. The detection itself is the trigger,
// so it must not depend on stdin being a terminal — agents run non-interactively.
func printAIHint(ctx context.Context, w io.Writer, args []string) {
	if os.Getenv("CI") != "" {
		return
	}
	// Shell completion (`__complete …`) must not run the skills.sh lookup: it
	// would block the TAB press and burn the daily hint slot.
	if len(args) > 0 && strings.HasPrefix(args[0], "__complete") {
		return
	}
	if noHint, err := rootCmd.PersistentFlags().GetBool("no-ai-hint"); err == nil && noHint {
		return
	}

	cmd, _, err := rootCmd.Find(args)
	if err != nil || cmd == nil || isAICommand(cmd) {
		return
	}
	skill := recommendedSkill(cmd)
	if skill == "" {
		return
	}

	_, msg := suggest(ctx, skill)
	if msg == "" {
		return
	}

	// Only suppress future recommendations once this one actually reached the user.
	if _, err := fmt.Fprintln(w, msg); err != nil {
		logging.FromContext(ctx).Debugf("could not write ai recommendation: %v", err)

		return
	}
	if err := recommend.MarkShown(); err != nil {
		logging.FromContext(ctx).Debugf("could not record ai recommendation: %v", err)
	}
}

// isAICommand reports whether cmd is under the `ai` group, so the hint does not
// nag while the user is already working with `ai`.
func isAICommand(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Name() == "ai" {
			return true
		}
	}

	return false
}

// recommendedSkill returns the skill cmd (or its nearest annotated parent)
// recommends, or "" when none is annotated.
func recommendedSkill(cmd *cobra.Command) string {
	for c := cmd; c != nil; c = c.Parent() {
		if s := c.Annotations[agent.RecommendedSkillAnnotation]; s != "" {
			return s
		}
	}

	return ""
}
