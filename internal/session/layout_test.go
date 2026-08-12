package session

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jonnyom/slis/internal/model"
)

func TestPlanGroupTabsUsesSharedRoot(t *testing.T) {
	root := t.TempDir()
	members := []model.SliceMember{
		{Repo: "api", WorktreePath: filepath.Join(root, "feature", "api")},
		{Repo: "web", WorktreePath: filepath.Join(root, "feature", "web")},
	}

	got := PlanGroupTabs(members, LayoutOptions{Root: root, Layout: "root"})
	want := []TabSpec{{ID: "root", Kind: TabKindRoot, Title: "root", CWD: filepath.Join(root, "feature")}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tabs = %#v, want %#v", got, want)
	}
}

func TestPlanGroupTabsCreatesSortedRepoTabs(t *testing.T) {
	root := t.TempDir()
	members := []model.SliceMember{
		{Repo: "web", WorktreePath: filepath.Join(root, "web")},
		{Repo: "api", WorktreePath: filepath.Join(root, "api")},
	}

	got := PlanGroupTabs(members, LayoutOptions{Layout: "repos"})
	want := []TabSpec{
		{ID: "repo-api", Kind: TabKindRepo, Title: "api", CWD: filepath.Join(root, "api")},
		{ID: "repo-web", Kind: TabKindRepo, Title: "web", CWD: filepath.Join(root, "web")},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tabs = %#v, want %#v", got, want)
	}
}

func TestPlanGroupTabsCreatesRootThenRepoTabs(t *testing.T) {
	root := t.TempDir()
	members := []model.SliceMember{
		{Repo: "web", WorktreePath: filepath.Join(root, "feature", "web")},
		{Repo: "api", WorktreePath: filepath.Join(root, "feature", "api")},
	}

	got := PlanGroupTabs(members, LayoutOptions{Root: root, Layout: "both"})
	want := []TabSpec{
		{ID: "root", Kind: TabKindRoot, Title: "root", CWD: filepath.Join(root, "feature")},
		{ID: "repo-api", Kind: TabKindRepo, Title: "api", CWD: filepath.Join(root, "feature", "api")},
		{ID: "repo-web", Kind: TabKindRepo, Title: "web", CWD: filepath.Join(root, "feature", "web")},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tabs = %#v, want %#v", got, want)
	}
}

func TestPlanGroupTabsDefaultsToRootWhenWorkspaceRootExists(t *testing.T) {
	root := t.TempDir()
	members := []model.SliceMember{
		{Repo: "web", WorktreePath: filepath.Join(root, "feature", "web")},
		{Repo: "api", WorktreePath: filepath.Join(root, "feature", "api")},
	}

	got := PlanGroupTabs(members, LayoutOptions{Root: root})
	want := []TabSpec{{ID: "root", Kind: TabKindRoot, Title: "root", CWD: filepath.Join(root, "feature")}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tabs = %#v, want %#v", got, want)
	}
}

func TestPlanGroupTabsFallsBackToReposForScatteredWorktrees(t *testing.T) {
	members := []model.SliceMember{
		{Repo: "web", WorktreePath: "/somewhere/web"},
		{Repo: "api", WorktreePath: "/elsewhere/api"},
	}

	got := PlanGroupTabs(members, LayoutOptions{Root: "/workspace", Layout: "both"})
	want := []TabSpec{
		{ID: "repo-api", Kind: TabKindRepo, Title: "api", CWD: "/elsewhere/api"},
		{ID: "repo-web", Kind: TabKindRepo, Title: "web", CWD: "/somewhere/web"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tabs = %#v, want %#v", got, want)
	}
}
