package session

import (
	"path/filepath"
	"sort"

	"github.com/jonnyom/slis/internal/model"
)

type TabKind string

const (
	TabKindRoot   TabKind = "root"
	TabKindRepo   TabKind = "repo"
	TabKindAgent  TabKind = "agent"
	TabKindShell  TabKind = "shell"
	TabKindReview TabKind = "review"
)

type TabSpec struct {
	ID    string
	Kind  TabKind
	Title string
	CWD   string
}

type LayoutOptions struct {
	Root   string
	Layout string
}

func PlanGroupTabs(members []model.SliceMember, options LayoutOptions) []TabSpec {
	if len(members) == 0 {
		return nil
	}
	sortedMembers := append([]model.SliceMember(nil), members...)
	sort.Slice(sortedMembers, func(left, right int) bool {
		return sortedMembers[left].Repo < sortedMembers[right].Repo
	})

	layout := options.Layout
	if layout == "" {
		if options.Root == "" {
			layout = "repos"
		} else {
			layout = "root"
		}
	}

	tabs := make([]TabSpec, 0, len(sortedMembers)+1)
	rootCWD, hasRootCWD := sharedRootCWD(sortedMembers)
	if layout != "repos" && options.Root != "" && hasRootCWD {
		tabs = append(tabs, TabSpec{
			ID:    "root",
			Kind:  TabKindRoot,
			Title: "root",
			CWD:   rootCWD,
		})
	}
	if layout == "root" && len(tabs) > 0 {
		return tabs
	}
	for _, member := range sortedMembers {
		tabs = append(tabs, TabSpec{
			ID:    "repo-" + member.Repo,
			Kind:  TabKindRepo,
			Title: member.Repo,
			CWD:   member.WorktreePath,
		})
	}
	return tabs
}

func sharedRootCWD(members []model.SliceMember) (string, bool) {
	if len(members) == 1 {
		return members[0].WorktreePath, true
	}
	parent := filepath.Dir(members[0].WorktreePath)
	for _, member := range members[1:] {
		if filepath.Dir(member.WorktreePath) != parent {
			return "", false
		}
	}
	return parent, true
}
