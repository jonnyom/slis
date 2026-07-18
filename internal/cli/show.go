package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/report"
	"github.com/spf13/cobra"
)

type (
	OrderedBranchDTO = report.OrderedBranchDTO
	MemberDetailDTO  = report.MemberDetailDTO
	SliceDetailDTO   = report.SliceDetailDTO
)

var buildDetail = report.BuildDetail

var showCmd = &cobra.Command{
	Use:   "show <slice>",
	Short: "Show details of a slice including per-repo gt stacks",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		useJSON, _ := cmd.Flags().GetBool("json")

		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}

		sp := config.StatePaths()
		dtos, err := listSlices(ws, sp.Overrides, sp.ActiveJournal)
		if err != nil {
			return err
		}

		var found *SliceDTO
		for i := range dtos {
			if dtos[i].Name == name {
				found = &dtos[i]
				break
			}
		}
		if found == nil {
			return fmt.Errorf("slice %q not found", name)
		}

		detail := buildDetail(*found)

		if useJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(detail)
		}

		printDetail(detail)
		return nil
	},
}

func printDetail(detail SliceDetailDTO) {
	active := ""
	if detail.Active {
		active = " (active)"
	}
	fmt.Printf("Slice: %s%s\n", detail.Name, active)
	if detail.Base != "" {
		fmt.Printf("Base:  %s\n", detail.Base)
	}
	fmt.Println()

	for _, m := range detail.Members {
		fmt.Printf("  %s  branch=%s  path=%s  sha=%s\n",
			m.Repo, m.Branch, m.WorktreePath, m.TipSHA)
		if len(m.Stack) > 0 {
			fmt.Println("    gt stack:")
			for _, ob := range m.Stack {
				indent := strings.Repeat("  ", ob.Depth+3)
				restack := ""
				if ob.NeedsRestack {
					restack = " [needs-restack]"
				}
				trunk := ""
				if ob.Trunk {
					trunk = " [trunk]"
				}
				fmt.Printf("%s%s%s%s\n", indent, ob.Name, trunk, restack)
			}
		}
		fmt.Println()
	}
}

func init() {
	showCmd.Flags().Bool("json", false, "Output as JSON")
}
