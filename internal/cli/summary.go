package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/diff"
	"github.com/jonnyom/slis/internal/report"
	"github.com/jonnyom/slis/internal/summary"
)

// RepoCommitsDTO is a JSON-friendly per-repo commit summary for a slice.
type RepoCommitsDTO struct {
	Repo    string   `json:"repo"`
	Branch  string   `json:"branch"`
	Commits []string `json:"commits"`
}

var findSlice = report.FindSlice

var summaryCmd = &cobra.Command{
	Use:   "summary <slice>",
	Short: "Show commit summary (or AI prose summary) for a slice",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sliceName := args[0]
		useAI, _ := cmd.Flags().GetBool("ai")
		useJSON, _ := cmd.Flags().GetBool("json")
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

		// --json is the deterministic, parseable twin: emit the same per-repo
		// commit subjects as the markdown path, never the --ai prose.
		if useJSON {
			byRepo, _ := summary.CommitSummary(sl, base)
			dtos := make([]RepoCommitsDTO, 0, len(sl.Members))
			for _, repo := range sl.Repos() {
				commits := byRepo[repo]
				if commits == nil {
					commits = []string{}
				}
				dtos = append(dtos, RepoCommitsDTO{
					Repo:    repo,
					Branch:  sl.Members[repo].Branch,
					Commits: commits,
				})
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(dtos)
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

		out, err := summary.AISummary(combined, summary.RunnerForHarness(ws.Sessions.HarnessName()))
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
	summaryCmd.Flags().Bool("json", false, "Output the per-repo commit summary as JSON (ignores --ai)")
	summaryCmd.Flags().String("base", "", "Base branch/ref to diff against (default: auto-detect each repo's trunk)")
}
