package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/radar"
	"github.com/jonnyom/slis/internal/report"
)

type ConflictsDTO = report.ConflictsDTO

var computeConflicts = report.ComputeConflicts

// renderConflicts writes a human-readable conflict report to w.
func renderConflicts(w io.Writer, idx *radar.Index) {
	if len(idx.Overlaps) == 0 {
		fmt.Fprintln(w, "No cross-slice conflicts — no file is changed by more than one slice.")
	} else {
		fmt.Fprintf(w, "%d file overlap(s) across slices (file touched by >1 slice — review before merge):\n\n", len(idx.Overlaps))
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "REPO\tFILE\tSLICES")
		for _, o := range idx.Overlaps {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", o.Repo, o.Path, strings.Join(o.Slices, ", "))
		}
		tw.Flush()
	}
	if len(idx.Incomplete) > 0 {
		fmt.Fprintf(w, "\nradar incomplete (diff unavailable, may hide conflicts) for: %s\n",
			strings.Join(idx.Incomplete, ", "))
	}
}

var conflictsCmd = &cobra.Command{
	Use:   "conflicts",
	Short: "Show files changed by more than one slice (cross-slice conflict radar)",
	Long: "Report files changed by more than one in-flight slice in the same repo — " +
		"a heads-up that two features may collide at merge time. File overlap is a " +
		"high-signal warning, not a guaranteed git conflict.",
	RunE: func(cmd *cobra.Command, args []string) error {
		useJSON, _ := cmd.Flags().GetBool("json")

		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}

		sp := config.StatePaths()
		idx, err := computeConflicts(ws, sp.Overrides)
		if err != nil {
			return err
		}

		if useJSON {
			dto := ConflictsDTO{Overlaps: idx.Overlaps, Incomplete: idx.Incomplete}
			if dto.Overlaps == nil {
				dto.Overlaps = []radar.Overlap{}
			}
			if dto.Incomplete == nil {
				dto.Incomplete = []string{}
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(dto)
		}

		renderConflicts(os.Stdout, idx)
		return nil
	},
}

func init() {
	conflictsCmd.Flags().Bool("json", false, "Output as JSON")
	rootCmd.AddCommand(conflictsCmd)
}
