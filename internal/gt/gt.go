// Package gt reads Graphite stack metadata via `gt state` (read-only parsing).
// The sole mutating operation is Restack (see restack.go), which rebases a
// stack and is always invoked behind an explicit user confirmation.
package gt

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"sort"
	"time"
)

// stateTimeout bounds a `gt state` read. gt is a Node process and can hang
// (e.g. on a slow filesystem or a stuck child), so cap it to keep a background
// concurrency slot from being held indefinitely.
const stateTimeout = 30 * time.Second

// Parent is a single entry in the parents array returned by `gt state`.
type Parent struct {
	Ref string `json:"ref"`
	SHA string `json:"sha"`
}

// BranchState holds the parsed per-branch fields from `gt state` output.
// Trunk branches may omit needs_restack and parents entirely.
type BranchState struct {
	Trunk        bool     `json:"trunk"`
	NeedsRestack bool     `json:"needs_restack"`
	Parents      []Parent `json:"parents"`
}

// State is the top-level map returned by `gt state`: branch name → BranchState.
type State map[string]BranchState

// OrderedBranch is a branch entry produced by State.Ordered for use in the TUI.
type OrderedBranch struct {
	Name         string
	Depth        int
	Trunk        bool
	NeedsRestack bool
}

// StripBanner returns a sub-slice of data beginning at the first '{' or '['
// character. gt prints a deprecation banner to stderr; stdout should be clean
// JSON, but this is a defensive guard. If neither '{' nor '[' is found, data
// is returned unchanged.
func StripBanner(data []byte) []byte {
	curly := bytes.IndexByte(data, '{')
	bracket := bytes.IndexByte(data, '[')

	switch {
	case curly == -1 && bracket == -1:
		return data
	case curly == -1:
		return data[bracket:]
	case bracket == -1:
		return data[curly:]
	default:
		if curly < bracket {
			return data[curly:]
		}
		return data[bracket:]
	}
}

// ParseState strips any banner prefix and unmarshals `gt state` JSON into a
// State map. Returns an error for malformed JSON.
func ParseState(data []byte) (State, error) {
	clean := StripBanner(data)
	var s State
	if err := json.Unmarshal(clean, &s); err != nil {
		return nil, err
	}
	return s, nil
}

// ReadState runs `gt state --no-interactive` in repoDir, capturing stdout
// only (the deprecation banner goes to stderr and is discarded). If gt is not
// installed, it returns an empty State and a nil error so callers can degrade
// gracefully.
func ReadState(repoDir string) (State, error) {
	if _, err := exec.LookPath("gt"); err != nil {
		// gt not installed — return gracefully rather than erroring.
		return State{}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), stateTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gt", "state", "--no-interactive")
	cmd.Dir = repoDir
	var out bytes.Buffer
	cmd.Stdout = &out
	// Discard stderr (banner / warnings go there).
	cmd.Stderr = nil

	_ = cmd.Run() // ignore exit-code errors; parse what we got
	return ParseState(out.Bytes())
}

// Ordered returns branches in a deterministic trunk-first, depth-ordered list.
// Within a given depth, branches are sorted by name. Branches unreachable from
// the trunk branch appear at the end, also sorted by name.
//
// It is safe against cycles (gt stacks are DAGs/trees in practice, but we
// guard via a visited set anyway).
func (s State) Ordered() []OrderedBranch {
	// Find the trunk branch.
	trunkName := ""
	for name, b := range s {
		if b.Trunk {
			trunkName = name
			break
		}
	}

	result := make([]OrderedBranch, 0, len(s))
	visited := make(map[string]bool, len(s))

	// BFS from trunk, assigning depths.
	type entry struct {
		name  string
		depth int
	}
	if trunkName != "" {
		queue := []entry{{trunkName, 0}}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			if visited[cur.name] {
				continue
			}
			visited[cur.name] = true

			b := s[cur.name]
			result = append(result, OrderedBranch{
				Name:         cur.name,
				Depth:        cur.depth,
				Trunk:        b.Trunk,
				NeedsRestack: b.NeedsRestack,
			})

			// Collect children (branches whose parent list contains cur.name).
			var children []string
			for childName, childState := range s {
				if visited[childName] {
					continue
				}
				for _, p := range childState.Parents {
					if p.Ref == cur.name {
						children = append(children, childName)
						break
					}
				}
			}
			sort.Strings(children)
			for _, child := range children {
				queue = append(queue, entry{child, cur.depth + 1})
			}
		}
	}

	// Append any branches unreachable from trunk, sorted by name.
	var unreachable []string
	for name := range s {
		if !visited[name] {
			unreachable = append(unreachable, name)
		}
	}
	sort.Strings(unreachable)
	for _, name := range unreachable {
		b := s[name]
		result = append(result, OrderedBranch{
			Name:         name,
			Trunk:        b.Trunk,
			NeedsRestack: b.NeedsRestack,
		})
	}

	return result
}

// Lineage returns only the stack connected to branch: the chain of ancestors
// from branch up to and including the trunk, plus branch's descendants. Branches
// belonging to other, unrelated stacks in the same repo's Graphite metadata are
// excluded — so a slice's Stack view shows the branches specific to that change,
// not every branch in the repo. Depth is assigned relative to the trunk
// (trunk = 0), matching Ordered.
//
// If branch is absent from the state, Lineage returns nil; the caller should
// then fall back to showing the branch name on its own.
func (s State) Lineage(branch string) []OrderedBranch {
	if _, ok := s[branch]; !ok {
		return nil
	}

	inLineage := map[string]bool{branch: true}

	// Ancestors: walk first-parent links from branch up to a trunk (or a branch
	// with no recorded parent). The visited guard tolerates malformed cycles.
	for cur := branch; ; {
		b, ok := s[cur]
		if !ok || b.Trunk || len(b.Parents) == 0 {
			break
		}
		parent := b.Parents[0].Ref
		if parent == "" || inLineage[parent] {
			break
		}
		inLineage[parent] = true
		cur = parent
	}

	// Descendants: BFS down from branch (any branch whose parent list names a
	// branch already in the lineage).
	for queue := []string{branch}; len(queue) > 0; {
		name := queue[0]
		queue = queue[1:]
		for child, cs := range s {
			if inLineage[child] {
				continue
			}
			for _, p := range cs.Parents {
				if p.Ref == name {
					inLineage[child] = true
					queue = append(queue, child)
					break
				}
			}
		}
	}

	// Emit in trunk-first, depth order by filtering the full ordering.
	out := make([]OrderedBranch, 0, len(inLineage))
	for _, b := range s.Ordered() {
		if inLineage[b.Name] {
			out = append(out, b)
		}
	}
	return out
}
