package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/diff"
	"github.com/jonnyom/slis/internal/discovery"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/summary"
	"github.com/jonnyom/slis/internal/swap"
)

// findSlice returns the model.Slice with the given name from the workspace, or
// an error if it cannot be found.
func findSlice(ws config.Workspace, name string) (model.Slice, error) {
	sp := config.StatePaths()

	slices, err := discovery.Discover(ws)
	if err != nil {
		return model.Slice{}, fmt.Errorf("discover: %w", err)
	}

	ov, _ := discovery.LoadOverrides(sp.Overrides)
	slices = discovery.Apply(slices, ov)

	j, _ := swap.Load(sp.ActiveJournal)
	for i, s := range slices {
		if j != nil && j.Slice == s.Name {
			slices[i].Active = true
		}
	}

	for _, s := range slices {
		if s.Name == name {
			return s, nil
		}
	}
	return model.Slice{}, fmt.Errorf("slice %q not found", name)
}

var summaryCmd = &cobra.Command{
	Use:   "summary <slice>",
	Short: "Show commit summary (or AI prose summary) for a slice",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sliceName := args[0]
		useAI, _ := cmd.Flags().GetBool("ai")
		base, _ := cmd.Flags().GetString("base")

		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}

		sl, err := findSlice(ws, sliceName)
		if err != nil {
			return err
		}

		// Respect an explicit --base override; otherwise use the slice's own Base.
		if !cmd.Flags().Changed("base") && sl.Base != "" {
			base = sl.Base
		}

		if !useAI {
			byRepo, _ := summary.CommitSummary(sl, base)
			md := summary.RenderCommitSummary(byRepo)
			fmt.Print(summary.RenderMarkdown(md))
			return nil
		}

		// --ai: build combined diff, call claude.
		diffs, _ := diff.SliceDiff(sl, base)
		var sb strings.Builder
		for _, rd := range diffs {
			fmt.Fprintf(&sb, "# repo: %s\n", rd.Repo)
			sb.WriteString(rd.Patch)
			if rd.Patch != "" && !strings.HasSuffix(rd.Patch, "\n") {
				sb.WriteString("\n")
			}
		}
		combined := sb.String()

		out, err := summary.AISummary(combined, summary.DefaultClaudeRunner)
		if err != nil {
			fmt.Printf("AI summary unavailable (%v); falling back to commit log:\n\n", err)
			byRepo, _ := summary.CommitSummary(sl, base)
			md := summary.RenderCommitSummary(byRepo)
			fmt.Print(summary.RenderMarkdown(md))
			return nil
		}

		fmt.Print(summary.RenderMarkdown(out))
		return nil
	},
}

func init() {
	summaryCmd.Flags().Bool("ai", false, "Use claude -p to generate an AI prose summary")
	summaryCmd.Flags().String("base", "", "Base branch/ref to diff against (default: auto-detect each repo's trunk)")
}
