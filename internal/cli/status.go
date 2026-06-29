package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/notify"
	"github.com/spf13/cobra"
)

// StatusDTO is a JSON-friendly representation of a slice's Claude session status.
type StatusDTO struct {
	Slice  string `json:"slice"`
	Status string `json:"status"`
}

// sliceStatuses returns the session status for every slice in the workspace,
// sorted by name. A slice with no recorded event reports "none". The slice set
// is the same canonical set ls shows (discovery + overrides), so the output is
// stable regardless of which slices happen to have event files.
func sliceStatuses(ws config.Workspace, sp config.Paths) ([]StatusDTO, error) {
	dtos, err := listSlices(ws, sp.Overrides, sp.ActiveJournal)
	if err != nil {
		return nil, err
	}

	out := make([]StatusDTO, 0, len(dtos))
	for _, s := range dtos {
		out = append(out, StatusDTO{
			Slice:  s.Name,
			Status: notify.ReadStatus(sp.EventsDir, s.Name).String(),
		})
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Slice < out[k].Slice })
	return out, nil
}

// renderStatusTable writes an aligned SLICE/STATUS table to w.
func renderStatusTable(w io.Writer, dtos []StatusDTO) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SLICE\tSTATUS")
	for _, dto := range dtos {
		fmt.Fprintf(tw, "%s\t%s\n", dto.Slice, dto.Status)
	}
	tw.Flush()
}

var statusCmd = &cobra.Command{
	Use:   "status [slice]",
	Short: "Show each slice's Claude session status (none/running/waiting-input/done)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		useJSON, _ := cmd.Flags().GetBool("json")
		sp := config.StatePaths()

		// Single slice: a direct event lookup — no workspace/discovery needed.
		if len(args) == 1 {
			dto := StatusDTO{
				Slice:  args[0],
				Status: notify.ReadStatus(sp.EventsDir, args[0]).String(),
			}
			if useJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(dto)
			}
			renderStatusTable(os.Stdout, []StatusDTO{dto})
			return nil
		}

		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}

		dtos, err := sliceStatuses(ws, sp)
		if err != nil {
			return err
		}

		if useJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(dtos)
		}

		renderStatusTable(os.Stdout, dtos)
		return nil
	},
}

func init() {
	statusCmd.Flags().Bool("json", false, "Output as JSON")
	rootCmd.AddCommand(statusCmd)
}
