package discovery

import "github.com/jonnyom/slis/internal/gt"

// GatherStack resolves the Graphite stack containing branch into a single
// representative tip plus the intermediate branches it subsumes.
//
// It walks the connected non-trunk component containing branch — every ancestor
// down to the trunk and every descendant above it — because a gather folds the
// whole stack regardless of which branch the user pointed at. (gt.State.Lineage
// is deliberately downstack-only, so gather does its own two-way walk.) The tip
// is the deepest branch: its commit contains every commit below it, so a slice
// represented by the tip carries the whole stack. The rest are folded away.
//
// ok is false when the branch is not part of a stack (fewer than two non-trunk
// branches in the component). linear is false when the component forks (a branch
// has more than one child in the stack), so the caller can warn that the gather
// pulled in a whole connected component rather than a single chain.
func GatherStack(st gt.State, branch string) (tip string, folded []string, linear bool, ok bool) {
	if _, present := st[branch]; !present {
		return "", nil, true, false
	}

	inStack := map[string]bool{}

	// Ancestors: first-parent links up to (not including) the trunk.
	for cur := branch; ; {
		b, exists := st[cur]
		if !exists || b.Trunk || inStack[cur] {
			break
		}
		inStack[cur] = true
		if len(b.Parents) == 0 {
			break
		}
		cur = b.Parents[0].Ref
	}

	// Descendants: BFS over children (any non-trunk branch whose parent is in
	// the stack).
	for queue := []string{branch}; len(queue) > 0; {
		name := queue[0]
		queue = queue[1:]
		for child, cs := range st {
			if inStack[child] || cs.Trunk {
				continue
			}
			for _, p := range cs.Parents {
				if p.Ref == name {
					inStack[child] = true
					queue = append(queue, child)
					break
				}
			}
		}
	}

	if len(inStack) < 2 {
		return "", nil, true, false
	}

	// The tip is the deepest branch in the component; the rest fold into it.
	// Ordered() is trunk-first by depth, so the last in-stack entry is the tip.
	ordered := make([]string, 0, len(inStack))
	for _, b := range st.Ordered() {
		if inStack[b.Name] {
			ordered = append(ordered, b.Name)
		}
	}
	tip = ordered[len(ordered)-1]
	folded = append([]string{}, ordered[:len(ordered)-1]...)

	return tip, folded, stackIsLinear(st, inStack), true
}

// stackIsLinear reports whether the stack forms a single chain — every branch in
// it has at most one child that is also in the stack. A branch with two in-stack
// children is a fork, so gathering pulled in siblings.
func stackIsLinear(st gt.State, inStack map[string]bool) bool {
	for name := range inStack {
		children := 0
		for child, cs := range st {
			if !inStack[child] {
				continue
			}
			for _, p := range cs.Parents {
				if p.Ref == name {
					children++
					break
				}
			}
		}
		if children > 1 {
			return false
		}
	}
	return true
}
