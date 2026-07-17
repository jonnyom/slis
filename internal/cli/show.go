package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/gt"
	"github.com/spf13/cobra"
)

// OrderedBranchDTO is a JSON-friendly copy of gt.OrderedBranch.
type OrderedBranchDTO struct {
	Name         string `json:"name"`
	Depth        int    `json:"depth"`
	Trunk        bool   `json:"trunk"`
	NeedsRestack bool   `json:"needs_restack"`
}

// MemberDetailDTO extends MemberDTO with a gt stack.
type MemberDetailDTO struct {
	MemberDTO
	Stack []OrderedBranchDTO `json:"stack,omitempty"`
}

// SliceDetailDTO extends SliceDTO with per-member stacks.
type SliceDetailDTO struct {
	Name    string            `json:"name"`
	Base    string            `json:"base"`
	Active  bool              `json:"active"`
	Members []MemberDetailDTO `json:"members"`
}

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

func buildDetail(dto SliceDTO) SliceDetailDTO {
	members := make([]MemberDetailDTO, 0, len(dto.Members))
	for _, m := range dto.Members {
		mdet := MemberDetailDTO{MemberDTO: m}
		if m.WorktreePath != "" {
			st, err := gt.ReadStack(m.WorktreePath)
			if err == nil {
				ordered := st.Ordered()
				stack := make([]OrderedBranchDTO, 0, len(ordered))
				for _, ob := range ordered {
					stack = append(stack, OrderedBranchDTO{
						Name:         ob.Name,
						Depth:        ob.Depth,
						Trunk:        ob.Trunk,
						NeedsRestack: ob.NeedsRestack,
					})
				}
				mdet.Stack = stack
			}
		}
		members = append(members, mdet)
	}
	return SliceDetailDTO{
		Name:    dto.Name,
		Base:    dto.Base,
		Active:  dto.Active,
		Members: members,
	}
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
