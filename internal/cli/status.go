package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/notify"
	"github.com/jonnyom/slis/internal/report"
	"github.com/spf13/cobra"
)

type StatusDTO = report.StatusDTO

var sliceStatuses = report.SliceStatuses

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
