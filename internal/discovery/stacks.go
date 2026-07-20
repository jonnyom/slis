package discovery

import (
	"github.com/jonnyom/slis/internal/gt"
	"github.com/jonnyom/slis/internal/model"
)

// StackReader reads a Graphite stack for a worktree. gt.ReadStack satisfies it;
// tests inject a fake so the annotation logic runs without the gt binary.
type StackReader func(worktreePath string) (gt.State, error)

// AnnotateStacks fills each slice's StackID/StackOrder from Graphite metadata so
// the UI can cluster stack-sibling slices together. For every slice it inspects
// members in sorted-repo order and uses the FIRST repo that yields a stack root
// for the member branch: StackID encodes that repo and root (so two slices are
// siblings only when they share a stack root in the SAME repo) and StackOrder is
// the branch's depth from that root. Slices with no Graphite data are left
// untouched (empty StackID) and fall back to flat-list behaviour.
//
// It mutates and returns slices in place. Slice identity and grouping are never
// changed — this is annotation only.
func AnnotateStacks(slices []model.Slice, read StackReader) []model.Slice {
	if read == nil {
		return slices
	}
	// gt state is repo-global: every worktree of a repo returns identical stack
	// metadata, so read it at most once per repo (each read spawns a gt process).
	stateByRepo := make(map[string]gt.State)
	for i := range slices {
		for _, repo := range slices[i].Repos() {
			m := slices[i].Members[repo]
			if m.WorktreePath == "" || m.Branch == "" {
				continue
			}
			st, cached := stateByRepo[repo]
			if !cached {
				var err error
				st, err = read(m.WorktreePath)
				if err != nil {
					st = nil
				}
				stateByRepo[repo] = st
			}
			if len(st) == 0 {
				continue
			}
			root, depth, ok := st.StackRoot(m.Branch)
			if !ok {
				continue
			}
			slices[i].StackID = repo + "\x00" + root
			slices[i].StackOrder = depth
			break
		}
	}
	return slices
}
