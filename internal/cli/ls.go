package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/report"
	"github.com/spf13/cobra"
)

// The JSON DTOs and builders live in internal/report so the cli and the rpc
// sidecar emit byte-for-byte identical shapes. These aliases and wrappers keep
// the cli call sites (and their tests) unchanged.
type (
	MemberDTO          = report.MemberDTO
	SliceDTO           = report.SliceDTO
	SkippedWorktreeDTO = report.SkippedWorktreeDTO
	RepoErrorDTO       = report.RepoErrorDTO
	CandidateDTO       = report.CandidateDTO
	MissingDTO         = report.MissingDTO
	LsResultDTO        = report.LsResultDTO
)

var (
	listSlices       = report.ListSlices
	listSlicesReport = report.ListSlicesReport
	registryPathFor  = report.RegistryPathFor
)

// renderSlicesTable writes an aligned table of slices to w.
func renderSlicesTable(w io.Writer, dtos []SliceDTO) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tACTIVE\tREPOS")
	for _, dto := range dtos {
		active := ""
		if dto.Active {
			active = "●"
			switch {
			case dto.Partial:
				active = "● partial"
			case dto.Stale:
				active = "● stale"
			}
		}
		repoSummaries := make([]string, 0, len(dto.Members))
		for _, m := range dto.Members {
			repoSummaries = append(repoSummaries, m.Repo+"("+m.Branch+")")
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", dto.Name, active, strings.Join(repoSummaries, ", "))
	}
	tw.Flush()
}

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List all slices in the workspace",
	RunE: func(cmd *cobra.Command, args []string) error {
		useJSON, _ := cmd.Flags().GetBool("json")

		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}

		sp := config.StatePaths()
		// Annotate stacks only for JSON consumers (the table view doesn't show
		// stack_id) so the human `slis ls` stays free of per-member gt reads.
		res, err := listSlicesReport(ws, sp.Overrides, sp.ActiveJournal, useJSON)
		if err != nil {
			return err
		}

		if useJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(res)
		}

		renderSlicesTable(os.Stdout, res.Slices)
		renderMissingTable(os.Stdout, res.Missing)
		renderSkippedNotice(os.Stderr, res)
		return nil
	},
}

// renderMissingTable prints registered slices whose worktree has gone missing,
// each flagged so a vanished slice is obvious (not silently absent).
func renderMissingTable(w io.Writer, missing []MissingDTO) {
	if len(missing) == 0 {
		return
	}
	fmt.Fprintln(w)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "MISSING\tREPO\tBRANCH\tPATH")
	for _, m := range missing {
		fmt.Fprintf(tw, "⚠ %s\t%s\t%s\t%s\n", m.Slice, m.Repo, m.Branch, m.Path)
	}
	tw.Flush()
}

// renderSkippedNotice prints a short warning to w (stderr) when discovery hid
// worktrees or a repo failed to list, pointing the user at `slis doctor`. It
// prints nothing when everything was healthy.
func renderSkippedNotice(w io.Writer, res LsResultDTO) {
	if len(res.Skipped) > 0 {
		reasons := make([]string, 0, len(res.Skipped))
		seen := map[string]bool{}
		for _, s := range res.Skipped {
			if !seen[s.Reason] {
				seen[s.Reason] = true
				reasons = append(reasons, s.Reason)
			}
		}
		sort.Strings(reasons)
		fmt.Fprintf(w, "⚠ %d worktree%s hidden (%s) — run slis doctor\n",
			len(res.Skipped), plural(len(res.Skipped)), strings.Join(reasons, "/"))
	}
	for _, e := range res.RepoErrors {
		fmt.Fprintf(w, "⚠ repo %q could not be read: %s — run slis doctor\n", e.Repo, e.Err)
	}
	if n := len(res.Candidates); n > 0 {
		fmt.Fprintf(w, "%d new worktree%s found — `slis candidates` to list, `slis import <path>` (or --all) to adopt\n",
			n, plural(n))
	}
}

func init() {
	lsCmd.Flags().Bool("json", false, "Output as JSON")
}
