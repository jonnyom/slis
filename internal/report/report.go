// Package report holds the read-only JSON builders shared by the cli and
// rpcserver packages. The DTO shapes here are the single source of truth for
// what `slis <cmd> --json` and the `slis rpc` sidecar both emit, so a JS or
// script client sees byte-for-byte identical structures from either surface.
//
// Everything in this package is read-only: it discovers slices, reads the swap
// journal, the notify event store, Graphite metadata, PRs, and diffs, but never
// mutates a repo.
package report

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/jonnyom/slis/internal/commentcache"
	"github.com/jonnyom/slis/internal/config"
	diffpkg "github.com/jonnyom/slis/internal/diff"
	"github.com/jonnyom/slis/internal/discovery"
	"github.com/jonnyom/slis/internal/forge"
	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/internal/gt"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/notify"
	"github.com/jonnyom/slis/internal/radar"
	"github.com/jonnyom/slis/internal/swap"
	"github.com/jonnyom/slis/internal/tmuxctl"
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
	Partial    bool        `json:"partial,omitempty"`
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

// OrderedBranchDTO is a JSON-friendly copy of gt.OrderedBranch.
type OrderedBranchDTO struct {
	Name         string `json:"name"`
	Depth        int    `json:"depth"`
	Trunk        bool   `json:"trunk"`
	NeedsRestack bool   `json:"needs_restack"`
	Added        *int   `json:"added,omitempty"`
	Deleted      *int   `json:"deleted,omitempty"`
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

// StatusDTO is a JSON-friendly representation of a slice's Claude session status.
type StatusDTO struct {
	Slice  string `json:"slice"`
	Status string `json:"status"`
}

// SliceStatus returns the semantic hook status when one has been recorded. A
// live tmux session is a conservative running fallback for older/broken hook
// installations, matching the legacy TUI instead of labelling active work idle.
func SliceStatus(eventsDir, slice string) model.SessionStatus {
	status := notify.ReadStatus(eventsDir, slice)
	if status == model.SessNone && tmuxctl.SessionExists(slice) {
		return model.SessRunning
	}
	return status
}

// PRStackRowDTO is a JSON-friendly representation of one repo's PR in a slice's
// stack. Number/URL/State/Title are empty when the branch has no PR. StackOrder
// is the branch's trunk-relative Graphite depth (1 = directly off trunk),
// omitted when the repo has no stack data; rows are ordered trunk-first by it.
// CI/CIPass/CIFail/CIPending carry the check rollup so a front-end can show a CI
// badge per row without a second fetch; they are omitted for a branch with no PR.
type PRStackRowDTO struct {
	Repo           string `json:"repo"`
	Branch         string `json:"branch"`
	Number         int    `json:"number,omitempty"`
	URL            string `json:"url,omitempty"`
	State          string `json:"state,omitempty"`
	Title          string `json:"title,omitempty"`
	ReviewDecision string `json:"review_decision,omitempty"`
	StackOrder     int    `json:"stack_order,omitempty"`
	CI             string `json:"ci,omitempty"`
	CIPass         int    `json:"ci_pass,omitempty"`
	CIFail         int    `json:"ci_fail,omitempty"`
	CIPending      int    `json:"ci_pending,omitempty"`
}

// SetPR fills a row's PR-derived fields (identity, review decision, and the CI
// check rollup) from a resolved PR. A nil pr leaves the row as a bare
// repo/branch entry, so callers can call it unconditionally.
func (r *PRStackRowDTO) SetPR(pr *forge.PR) {
	if pr == nil {
		return
	}
	r.Number = pr.Number
	r.URL = pr.URL
	r.State = pr.State
	r.Title = pr.Title
	r.ReviewDecision = pr.ReviewDecision
	overall, pass, fail, pending := pr.CISummary()
	r.CI = forge.CIStateName(overall)
	r.CIPass, r.CIFail, r.CIPending = pass, fail, pending
}

// ConflictsDTO is the JSON shape for `slis conflicts --json`.
type ConflictsDTO struct {
	Overlaps   []radar.Overlap `json:"overlaps"`
	Incomplete []string        `json:"incomplete"`
}

// RegistryPathFor returns the managed-slice registry path that sits beside the
// given overrides file, so a caller that passes an isolated overrides path (a
// test) automatically gets an isolated registry too.
func RegistryPathFor(overridesPath string) string {
	return filepath.Join(filepath.Dir(overridesPath), "registry.yaml")
}

// ListSlices loads all slices from the workspace, applies overrides, marks the
// active slice from the journal, and returns DTOs sorted by name. Overrides and
// journal paths that do not exist are silently treated as empty/absent.
func ListSlices(ws config.Workspace, overridesPath, journalPath string) ([]SliceDTO, error) {
	res, err := ListSlicesReport(ws, overridesPath, journalPath, false)
	if err != nil {
		return nil, err
	}
	return res.Slices, nil
}

// ListSlicesReport is ListSlices plus the skipped-worktree / repo-error /
// candidate / missing report from discovery, so callers that surface hidden or
// unmanaged worktrees (ls, doctor, TUI, rpc) can report them. Ingestion is
// opt-in: only registered (or managed-tree) worktrees become slices; the rest
// are candidates.
func ListSlicesReport(ws config.Workspace, overridesPath, journalPath string, annotateStacks bool) (LsResultDTO, error) {
	rep := discovery.Report(ws, RegistryPathFor(overridesPath))

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
		dto := ToSliceDTO(s)
		if j != nil && j.Slice == s.Name {
			dto.Active = true
			if len(swap.StaleRepos(j, tipByRepo(s))) > 0 {
				dto.Stale = true
			}
			if journalPartial(j, s) {
				dto.Partial = true
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

// journalPartial reports whether the active journal covers only a subset of the
// slice's members — i.e. at least one member repo has no journal entry, so the
// slice is only partly swapped in (a crash mid-activate left the rest behind).
func journalPartial(j *swap.Journal, s model.Slice) bool {
	covered := make(map[string]bool, len(j.Repos))
	for _, rs := range j.Repos {
		covered[rs.Repo] = true
	}
	for repo := range s.Members {
		if !covered[repo] {
			return true
		}
	}
	return false
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

// ToSliceDTO converts a model.Slice to a SliceDTO. Members are sorted by Repo
// for stable output.
func ToSliceDTO(s model.Slice) SliceDTO {
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

// BuildDetail turns a SliceDTO into a SliceDetailDTO by reading each member's
// Graphite stack from its worktree (best-effort; a repo with no gt data yields
// an empty stack).
func BuildDetail(dto SliceDTO) SliceDetailDTO {
	members := make([]MemberDetailDTO, 0, len(dto.Members))
	for _, m := range dto.Members {
		mdet := MemberDetailDTO{MemberDTO: m}
		if m.WorktreePath != "" {
			st, err := gt.ReadStack(m.WorktreePath)
			if err == nil {
				dirtyAdded, dirtyDeleted, dirtyOK := memberDirtyCounts(m)
				// A slice owns the branch checked out in this worktree. Include only
				// its downstack ancestry for context; siblings and upstack branches
				// belong to other worktrees/slices and must never leak into this view.
				ordered := st.Lineage(m.Branch)
				stack := make([]OrderedBranchDTO, 0, len(ordered)+1)
				sawCurrent := false
				sawTrunk := false
				maxDepth := -1
				for _, ob := range ordered {
					if ob.Name == m.Branch {
						sawCurrent = true
					}
					if ob.Depth > maxDepth {
						maxDepth = ob.Depth
					}
					if ob.Trunk {
						sawTrunk = true
					}
					node := OrderedBranchDTO{
						Name:         ob.Name,
						Depth:        ob.Depth,
						Trunk:        ob.Trunk,
						NeedsRestack: ob.NeedsRestack,
					}
					switch {
					case ob.Trunk:
						// Trunk has no stack parent, so a +/- comparison is undefined.
					case ob.Name == m.Branch && dirtyOK:
						node.Added, node.Deleted = intPtr(dirtyAdded), intPtr(dirtyDeleted)
					default:
						if bd, err := BranchDiff(m.WorktreePath, m.Repo, ob.Name, "stat"); err == nil && bd.Err == "" && bd.Stat != nil {
							node.Added, node.Deleted = intPtr(bd.Stat.Added), intPtr(bd.Stat.Deleted)
						}
					}
					stack = append(stack, node)
				}
				// An untracked or partially-recorded Graphite branch has no lineage,
				// but Git can still provide the repository trunk. Preserve the same
				// trunk → member structure every repo group uses without mutating gt.
				if !sawTrunk {
					base := git.DetectBase(m.WorktreePath)
					if base != "" && git.RefExists(m.WorktreePath, base) {
						if base == m.Branch {
							if !sawCurrent {
								stack = append(stack, OrderedBranchDTO{Name: base, Trunk: true})
								sawCurrent = true
							} else {
								for i := range stack {
									if stack[i].Name == m.Branch {
										stack[i].Trunk = true
										stack[i].Depth = 0
									}
								}
							}
							maxDepth = 0
						} else {
							for i := range stack {
								stack[i].Depth++
							}
							stack = append([]OrderedBranchDTO{{Name: base, Trunk: true}}, stack...)
							maxDepth++
						}
					}
				}
				// A newly-created/untracked Graphite branch may be absent from gt's
				// lineage even though it is the worktree's actual branch. Never hide
				// the member: append it after the known lineage as a best-effort leaf.
				if !sawCurrent {
					node := OrderedBranchDTO{Name: m.Branch, Depth: maxDepth + 1}
					if dirtyOK {
						node.Added, node.Deleted = intPtr(dirtyAdded), intPtr(dirtyDeleted)
					}
					stack = append(stack, node)
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

func intPtr(n int) *int { return &n }

// memberDirtyCounts matches the cockpit's default "working" scope for the
// currently checked-out member branch. Other stack rows use committed
// branch-vs-parent counts above.
func memberDirtyCounts(m MemberDTO) (added, deleted int, ok bool) {
	sl := model.Slice{Members: map[string]model.SliceMember{
		m.Repo: {
			Repo:         m.Repo,
			Branch:       m.Branch,
			WorktreePath: m.WorktreePath,
			TipSHA:       m.TipSHA,
		},
	}}
	diffs, err := diffpkg.SliceDirtyStat(sl)
	if err != nil || len(diffs) != 1 || diffs[0].Err != "" {
		return 0, 0, false
	}
	return diffs[0].TotalAdded(), diffs[0].TotalDeleted(), true
}

// SliceStatuses returns the session status for every slice in the workspace,
// sorted by name. Without a recorded event, a live tmux session reports
// "running" and a slice with no session reports "none". The slice set is the
// same canonical set ls shows (discovery + overrides).
func SliceStatuses(ws config.Workspace, sp config.Paths) ([]StatusDTO, error) {
	dtos, err := ListSlices(ws, sp.Overrides, sp.ActiveJournal)
	if err != nil {
		return nil, err
	}

	out := make([]StatusDTO, 0, len(dtos))
	for _, s := range dtos {
		out = append(out, StatusDTO{Slice: s.Name, Status: SliceStatus(sp.EventsDir, s.Name).String()})
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Slice < out[k].Slice })
	return out, nil
}

// FindSlice returns the model.Slice with the given name from the workspace, or
// an error if it cannot be found. It marks the returned slice Active from the
// swap journal. Paths come from the global state dir; FindSliceIn takes them
// explicitly.
func FindSlice(ws config.Workspace, name string) (model.Slice, error) {
	return FindSliceIn(ws, config.StatePaths(), name)
}

// FindSliceIn is FindSlice with explicit state paths, so a caller with its own
// (e.g. the rpc sidecar, or a test) resolves against those rather than the
// global state dir.
func FindSliceIn(ws config.Workspace, sp config.Paths, name string) (model.Slice, error) {
	slices := discovery.Report(ws, sp.Registry).Slices

	ov, _ := discovery.LoadOverrides(sp.Overrides)
	slices = discovery.Apply(slices, ov)

	j, _ := swap.Load(sp.ActiveJournal)
	for i, s := range slices {
		if j != nil && j.Slice == s.Name {
			slices[i].Active = true
		}
	}

	for _, s := range slices {
		if s.Name == name {
			return s, nil
		}
	}
	return model.Slice{}, fmt.Errorf("slice %q not found", name)
}

// StackDepths returns each member repo's trunk-relative Graphite depth for the
// member branch, and whether ANY repo yielded stack data. Depth 0 means the
// branch was not found in that repo's stack (no data); real stacked branches sit
// at depth ≥ 1 (trunk = 0).
func StackDepths(sl model.Slice) (depths map[string]int, anyStack bool) {
	depths = make(map[string]int, len(sl.Members))
	for _, repo := range sl.Repos() {
		m := sl.Members[repo]
		if m.WorktreePath == "" {
			continue
		}
		st, err := gt.ReadStack(m.WorktreePath)
		if err != nil || len(st) == 0 {
			continue
		}
		for _, ob := range st.Ordered() {
			if ob.Name == m.Branch {
				depths[repo] = ob.Depth
				anyStack = true
				break
			}
		}
	}
	return depths, anyStack
}

// OrderReposByStack returns the slice's repos ordered trunk-first by Graphite
// depth (ties broken by name) when stack data is available, else the plain
// alphabetical order sl.Repos() already provides.
func OrderReposByStack(sl model.Slice, depths map[string]int, anyStack bool) []string {
	repos := sl.Repos() // alphabetical
	if !anyStack {
		return repos
	}
	sort.SliceStable(repos, func(a, b int) bool {
		if depths[repos[a]] != depths[repos[b]] {
			return depths[repos[a]] < depths[repos[b]]
		}
		return repos[a] < repos[b]
	})
	return repos
}

// PRStackRows builds the `slis pr-stack --json` rows for a slice: one row per
// member repo, trunk-first by Graphite depth, each carrying the repo's PR (when
// one exists). Per-repo PR lookups that fail or find no PR leave the PR fields
// empty rather than aborting the whole slice.
func PRStackRows(sl model.Slice) []PRStackRowDTO {
	depths, anyStack := StackDepths(sl)
	repos := OrderReposByStack(sl, depths, anyStack)
	rows := make([]PRStackRowDTO, 0, len(repos))
	for _, repo := range repos {
		m := sl.Members[repo]
		row := PRStackRowDTO{Repo: repo, Branch: m.Branch, StackOrder: depths[repo]}
		pr, _ := forge.PRForBranch(m.WorktreePath, m.Branch)
		row.SetPR(pr)
		rows = append(rows, row)
	}
	return rows
}

// ComputeConflicts discovers slices, applies overrides, and builds the radar
// index over their changed-file sets. Stats are computed fresh (no TUI card
// cache outside the running program), concurrently per slice.
func ComputeConflicts(ws config.Workspace, overridesPath string) (*radar.Index, error) {
	slices := discovery.Report(ws, RegistryPathFor(overridesPath)).Slices
	ov, _ := discovery.LoadOverrides(overridesPath)
	slices = discovery.Apply(slices, ov)

	return radar.Build(radar.CollectStats(slices)), nil
}

// Conflicts is ComputeConflicts wrapped as a ready-to-marshal ConflictsDTO, with
// nil slices normalised to empty arrays so the JSON always has the two keys.
func Conflicts(ws config.Workspace, overridesPath string) (ConflictsDTO, error) {
	idx, err := ComputeConflicts(ws, overridesPath)
	if err != nil {
		return ConflictsDTO{}, err
	}
	dto := ConflictsDTO{Overlaps: idx.Overlaps, Incomplete: idx.Incomplete}
	if dto.Overlaps == nil {
		dto.Overlaps = []radar.Overlap{}
	}
	if dto.Incomplete == nil {
		dto.Incomplete = []string{}
	}
	return dto, nil
}

// Comments loads the cached PR comments for a slice (or every cached slice when
// all is true). It errors when a specific slice has no cached comments, matching
// `slis comments <slice>`.
func Comments(commentsPath, slice string, all bool) (commentcache.Store, error) {
	store, err := commentcache.Load(commentsPath)
	if err != nil {
		return nil, err
	}
	if slice != "" && !all {
		if _, ok := store[slice]; !ok {
			return nil, fmt.Errorf("no cached comments for slice %q", slice)
		}
		return commentcache.Store{slice: store[slice]}, nil
	}
	return store, nil
}
