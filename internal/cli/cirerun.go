package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/forge"
)

// ciRerunRow is one repo's outcome from re-triggering its PR's failed CI runs.
// n is how many GitHub Actions runs were re-triggered; err is set when the
// re-run failed for that repo (gh absent, gh error, …).
type ciRerunRow struct {
	repo string
	n    int
	err  error
}

// renderCIRerun formats the per-repo re-run outcomes as one line each. Pure, so
// it is unit-testable without hitting gh.
func renderCIRerun(rows []ciRerunRow) string {
	if len(rows) == 0 {
		return "no PRs with failing CI to re-run\n"
	}
	var sb strings.Builder
	for _, r := range rows {
		switch {
		case r.err != nil:
			fmt.Fprintf(&sb, "%s: error: %s\n", r.repo, r.err)
		case r.n == 0:
			fmt.Fprintf(&sb, "%s: no failing runs to re-trigger\n", r.repo)
		default:
			fmt.Fprintf(&sb, "%s: re-triggered %d run(s)\n", r.repo, r.n)
		}
	}
	return sb.String()
}

// ciRerunAnySucceeded reports whether at least one repo re-triggered a run.
func ciRerunAnySucceeded(rows []ciRerunRow) bool {
	for _, r := range rows {
		if r.err == nil && r.n > 0 {
			return true
		}
	}
	return false
}

var ciRerunCmd = &cobra.Command{
	Use:   "ci-rerun <slice>",
	Short: "Re-trigger the failed CI runs for each repo's PR in a slice",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sliceName := args[0]

		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}

		sl, err := findSlice(ws, sliceName)
		if err != nil {
			return err
		}

		var rows []ciRerunRow
		for _, repo := range sl.Repos() {
			m := sl.Members[repo]
			pr, _ := forge.PRForBranch(m.WorktreePath, m.Branch) // swallow per-repo lookup errors
			if pr == nil {
				continue
			}
			n, rerunErr := forge.RerunFailedChecks(m.WorktreePath, pr)
			rows = append(rows, ciRerunRow{repo: repo, n: n, err: rerunErr})
		}

		fmt.Print(renderCIRerun(rows))

		// Surface a non-zero exit only when nothing was re-triggered AND at least
		// one repo errored, so the JS mutation runner reports the failure.
		if !ciRerunAnySucceeded(rows) {
			for _, r := range rows {
				if r.err != nil {
					return fmt.Errorf("ci-rerun failed: %w", r.err)
				}
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(ciRerunCmd)
}
