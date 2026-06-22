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
	"github.com/jonnyom/slis/internal/discovery"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/swap"
	"github.com/spf13/cobra"
)

// MemberDTO is a JSON-friendly representation of a single slice member.
type MemberDTO struct {
	Repo         string `json:"repo"`
	Branch       string `json:"branch"`
	WorktreePath string `json:"worktree_path"`
	TipSHA       string `json:"tip_sha"`
}

// SliceDTO is a JSON-friendly representation of a slice.
type SliceDTO struct {
	Name    string      `json:"name"`
	Base    string      `json:"base"`
	Active  bool        `json:"active"`
	Members []MemberDTO `json:"members"`
}

// listSlices loads all slices from the workspace, applies overrides, marks the
// active slice from the journal, and returns DTOs sorted by name. Overrides and
// journal paths that do not exist are silently treated as empty/absent.
func listSlices(ws config.Workspace, overridesPath, journalPath string) ([]SliceDTO, error) {
	slices, err := discovery.Discover(ws)
	if err != nil {
		return nil, fmt.Errorf("discover: %w", err)
	}

	ov, _ := discovery.LoadOverrides(overridesPath)
	slices = discovery.Apply(slices, ov)

	j, _ := swap.Load(journalPath)

	dtos := make([]SliceDTO, 0, len(slices))
	for _, s := range slices {
		dto := toSliceDTO(s)
		if j != nil && j.Slice == s.Name {
			dto.Active = true
		}
		dtos = append(dtos, dto)
	}

	sort.Slice(dtos, func(i, k int) bool {
		return dtos[i].Name < dtos[k].Name
	})

	return dtos, nil
}

// toSliceDTO converts a model.Slice to a SliceDTO. Members are sorted by Repo
// for stable output.
func toSliceDTO(s model.Slice) SliceDTO {
	repos := s.Repos() // already sorted
	members := make([]MemberDTO, 0, len(repos))
	for _, repo := range repos {
		m := s.Members[repo]
		members = append(members, MemberDTO{
			Repo:         m.Repo,
			Branch:       m.Branch,
			WorktreePath: m.WorktreePath,
			TipSHA:       m.TipSHA,
		})
	}
	return SliceDTO{
		Name:    s.Name,
		Base:    s.Base,
		Active:  s.Active,
		Members: members,
	}
}

// renderSlicesTable writes an aligned table of slices to w.
func renderSlicesTable(w io.Writer, dtos []SliceDTO) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tACTIVE\tREPOS")
	for _, dto := range dtos {
		active := ""
		if dto.Active {
			active = "●"
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
		dtos, err := listSlices(ws, sp.Overrides, sp.ActiveJournal)
		if err != nil {
			return err
		}

		if useJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(dtos)
		}

		renderSlicesTable(os.Stdout, dtos)
		return nil
	},
}

func init() {
	lsCmd.Flags().Bool("json", false, "Output as JSON")
}
