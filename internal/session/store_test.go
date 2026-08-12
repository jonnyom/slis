package session

import (
	"reflect"
	"testing"
)

func TestStoreReloadsTabOrderAndActiveTab(t *testing.T) {
	directory := t.TempDir()
	want := Group{
		ID:          "feature-abc",
		ActiveTabID: "repo-web",
		Tabs: []Tab{
			{ID: "root", Kind: TabKindRoot, Title: "root", CWD: "/work/feature"},
			{ID: "repo-web", Kind: TabKindRepo, Title: "web", CWD: "/work/feature/web"},
		},
	}

	if err := OpenStore(directory).Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := OpenStore(directory).Get(want.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("group = %#v, want %#v", got, want)
	}
}

func TestStoreListReturnsGroupsSortedByID(t *testing.T) {
	store := OpenStore(t.TempDir())
	for _, group := range []Group{{ID: "zeta"}, {ID: "alpha"}} {
		if err := store.Save(group); err != nil {
			t.Fatal(err)
		}
	}

	groups, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	want := []Group{{ID: "alpha"}, {ID: "zeta"}}
	if !reflect.DeepEqual(groups, want) {
		t.Fatalf("groups = %#v, want %#v", groups, want)
	}
}
