package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/discovery"
	"github.com/jonnyom/slis/internal/gt"
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

// SliceDTO is a JSON-friendly representation of a slice. StackID/StackOrder are
// the optional Graphite stack annotations (present only when stack data is
// available and requested): slices sharing a StackID are stack siblings, ordered
// trunk-first by StackOrder.
type SliceDTO struct {
	Name       string      `json:"name"`
	Base       string      `json:"base"`
	Active     bool        `json:"active"`
	Stale      bool        `json:"stale"`
	Members    []MemberDTO `json:"members"`
	StackID    string      `json:"stack_id,omitempty"`
	StackOrder int         `json:"stack_order,omitempty"`
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

// CandidateDTO is a JSON-friendly worktree that slis found but did NOT ingest as
// a slice (opt-in): import it with `slis import` or hide it with `slis ignore`.
type CandidateDTO struct {
	Repo   string `json:"repo"`
	Path   string `json:"path"`
	Branch string `json:"branch"`
	Slice  string `json:"slice"`
}

// MissingDTO is a JSON-friendly registered slice member whose worktree no longer
// exists (or moved off its branch), so a known slice never silently vanishes.
type MissingDTO struct {
	Slice  string `json:"slice"`
	Repo   string `json:"repo"`
	Path   string `json:"path"`
	Branch string `json:"branch"`
}

// LsResultDTO is the top-level `slis ls --json` payload: the slices plus the
// worktrees that were skipped, the repos that failed to list, unmanaged
// candidate worktrees, and registered-but-missing members — so no worktree
// disappears (or appears) without explanation.
type LsResultDTO struct {
	Slices     []SliceDTO           `json:"slices"`
	Skipped    []SkippedWorktreeDTO `json:"skipped,omitempty"`
	RepoErrors []RepoErrorDTO       `json:"repo_errors,omitempty"`
	Candidates []CandidateDTO       `json:"candidates,omitempty"`
	Missing    []MissingDTO         `json:"missing,omitempty"`
}

// listSlices loads all slices from the workspace, applies overrides, marks the
// active slice from the journal, and returns DTOs sorted by name. Overrides and
// journal paths that do not exist are silently treated as empty/absent.
func listSlices(ws config.Workspace, overridesPath, journalPath string) ([]SliceDTO, error) {
	res, err := listSlicesReport(ws, overridesPath, journalPath, false)
	if err != nil {
		return nil, err
	}
	return res.Slices, nil
}

// registryPathFor returns the managed-slice registry path that sits beside the
// given overrides file, so a caller that passes an isolated overrides path (a
// test) automatically gets an isolated registry too.
func registryPathFor(overridesPath string) string {
	return filepath.Join(filepath.Dir(overridesPath), "registry.yaml")
}

// listSlicesReport is listSlices plus the skipped-worktree / repo-error /
// candidate / missing report from discovery, so callers that surface hidden or
// unmanaged worktrees (ls, doctor, TUI) can report them. Ingestion is opt-in:
// only registered (or managed-tree) worktrees become slices; the rest are
// candidates.
func listSlicesReport(ws config.Workspace, overridesPath, journalPath string, annotateStacks bool) (LsResultDTO, error) {
	rep := discovery.Report(ws, registryPathFor(overridesPath))

	ov, _ := discovery.LoadOverrides(overridesPath)
	slices := discovery.Apply(rep.Slices, ov)

	// Graphite stack annotation is opt-in: only ls --json needs it, and it costs
	// a `gt state` read per member, so polling commands (status) skip it.
	if annotateStacks {
		slices = discovery.AnnotateStacks(slices, gt.ReadStack)
	}

	j, _ := swap.Load(journalPath)

	dtos := make([]SliceDTO, 0, len(slices))
	for _, s := range slices {
		dto := toSliceDTO(s)
		if j != nil && j.Slice == s.Name {
			dto.Active = true
			if len(swap.StaleRepos(j, tipByRepo(s))) > 0 {
				dto.Stale = true
			}
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
	for _, c := range rep.Candidates {
		result.Candidates = append(result.Candidates, CandidateDTO(c))
	}
	for _, mm := range rep.Missing {
		result.Missing = append(result.Missing, MissingDTO(mm))
	}
	return result, nil
}

// tipByRepo maps each member's repo name to its current branch tip SHA, for
// staleness comparison against the swap journal's recorded TargetSHA.
func tipByRepo(s model.Slice) map[string]string {
	m := make(map[string]string, len(s.Members))
	for repo, mem := range s.Members {
		m[repo] = mem.TipSHA
	}
	return m
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
		Name:       s.Name,
		Base:       s.Base,
		Active:     s.Active,
		Members:    members,
		StackID:    s.StackID,
		StackOrder: s.StackOrder,
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
			if dto.Stale {
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
