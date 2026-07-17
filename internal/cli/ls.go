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

// SkippedWorktreeDTO is a JSON-friendly representation of a worktree discovery
// could not turn into a slice member, and why.
type SkippedWorktreeDTO struct {
	Repo   string `json:"repo"`
	Path   string `json:"path"`
	Branch string `json:"branch,omitempty"`
	Reason string `json:"reason"`
}

// RepoErrorDTO is a JSON-friendly representation of a repo whose worktree
// listing failed entirely.
type RepoErrorDTO struct {
	Repo string `json:"repo"`
	Err  string `json:"error"`
}

// LsResultDTO is the top-level `slis ls --json` payload: the slices plus the
// worktrees that were skipped and the repos that failed to list, so no worktree
// disappears without explanation.
type LsResultDTO struct {
	Slices     []SliceDTO           `json:"slices"`
	Skipped    []SkippedWorktreeDTO `json:"skipped,omitempty"`
	RepoErrors []RepoErrorDTO       `json:"repo_errors,omitempty"`
}

// listSlices loads all slices from the workspace, applies overrides, marks the
// active slice from the journal, and returns DTOs sorted by name. Overrides and
// journal paths that do not exist are silently treated as empty/absent.
func listSlices(ws config.Workspace, overridesPath, journalPath string) ([]SliceDTO, error) {
	res, err := listSlicesReport(ws, overridesPath, journalPath)
	if err != nil {
		return nil, err
	}
	return res.Slices, nil
}

// listSlicesReport is listSlices plus the skipped-worktree / repo-error report
// from discovery, so callers that surface hidden worktrees (ls, doctor, TUI)
// can report them.
func listSlicesReport(ws config.Workspace, overridesPath, journalPath string) (LsResultDTO, error) {
	rep := discovery.DiscoverReport(ws)

	ov, _ := discovery.LoadOverrides(overridesPath)
	slices := discovery.Apply(rep.Slices, ov)

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

	result := LsResultDTO{Slices: dtos}
	for _, s := range rep.Skipped {
		result.Skipped = append(result.Skipped, SkippedWorktreeDTO(s))
	}
	for _, e := range rep.RepoErrors {
		result.RepoErrors = append(result.RepoErrors, RepoErrorDTO(e))
	}
	return result, nil
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
		res, err := listSlicesReport(ws, sp.Overrides, sp.ActiveJournal)
		if err != nil {
			return err
		}

		if useJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(res)
		}

		renderSlicesTable(os.Stdout, res.Slices)
		renderSkippedNotice(os.Stderr, res)
		return nil
	},
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
}

func init() {
	lsCmd.Flags().Bool("json", false, "Output as JSON")
}
