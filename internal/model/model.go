package model

import "sort"

// SessionStatus represents the status of a Claude Code session running in a slice.
type SessionStatus int

const (
	SessNone SessionStatus = iota
	SessRunning
	SessWaitingInput
	SessDone
)

// String returns a stable lowercase string representation of the status.
func (s SessionStatus) String() string {
	switch s {
	case SessNone:
		return "none"
	case SessRunning:
		return "running"
	case SessWaitingInput:
		return "waiting-input"
	case SessDone:
		return "done"
	default:
		return "unknown"
	}
}

// SliceMember holds the git state for one repo within a slice.
type SliceMember struct {
	Repo, Branch, WorktreePath, TipSHA string
}

// Slice represents a named set of worktrees, one per repo, all sharing the same feature branch.
type Slice struct {
	Name, Base string
	Members    map[string]SliceMember // keyed by repo name
	Active     bool                   // currently swapped into primary

	// StackID and StackOrder are optional Graphite annotations (set after
	// discovery by discovery.AnnotateStacks, empty otherwise). Slices sharing a
	// StackID descend from the same stack root in the same repo — they are
	// stack siblings. StackOrder is the depth from that root (root = 0), giving
	// a trunk-first ordering within the stack. They are annotation only: slice
	// identity and grouping are never changed by them.
	StackID    string
	StackOrder int
}

// Repos returns the member repo names in sorted order.
func (s Slice) Repos() []string {
	repos := make([]string, 0, len(s.Members))
	for k := range s.Members {
		repos = append(repos, k)
	}
	sort.Strings(repos)
	return repos
}
