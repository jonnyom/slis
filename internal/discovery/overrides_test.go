package discovery

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jonnyom/slis/internal/model"
)

func TestApplyRegroups(t *testing.T) {
	discovered := []model.Slice{
		{
			Name: "checkout",
			Members: map[string]model.SliceMember{
				"web": {
					Repo:         "web",
					Branch:       "jonny/checkout",
					WorktreePath: "/wt/web",
					TipSHA:       "abc",
				},
			},
		},
		{
			Name: "checkout-api",
			Members: map[string]model.SliceMember{
				"api": {
					Repo:         "api",
					Branch:       "jonny/checkout-api",
					WorktreePath: "/wt/api",
					TipSHA:       "def",
				},
			},
		},
	}

	ov := Overrides{
		"checkout": {
			"web": "jonny/checkout",
			"api": "jonny/checkout-api",
		},
	}

	got := Apply(discovered, ov)

	// Expect exactly one slice named "checkout".
	if len(got) != 1 {
		t.Fatalf("expected 1 slice, got %d", len(got))
	}
	if got[0].Name != "checkout" {
		t.Fatalf("expected slice name %q, got %q", "checkout", got[0].Name)
	}

	// Expect both web and api members.
	if len(got[0].Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(got[0].Members))
	}

	// Verify web member is intact.
	web, ok := got[0].Members["web"]
	if !ok {
		t.Fatal("expected member 'web' in checkout slice")
	}
	if web.WorktreePath != "/wt/web" || web.TipSHA != "abc" {
		t.Errorf("web member unexpected: %+v", web)
	}

	// Verify api member was moved and kept its original fields.
	api, ok := got[0].Members["api"]
	if !ok {
		t.Fatal("expected member 'api' in checkout slice")
	}
	if api.WorktreePath != "/wt/api" {
		t.Errorf("api WorktreePath: want %q, got %q", "/wt/api", api.WorktreePath)
	}
	if api.TipSHA != "def" {
		t.Errorf("api TipSHA: want %q, got %q", "def", api.TipSHA)
	}
}

func TestSaveLoadOverridesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.yaml")
	ov := Overrides{
		"checkout": {
			"web": "jonny/checkout",
			"api": "jonny/checkout-api",
		},
	}

	if err := SaveOverrides(path, ov); err != nil {
		t.Fatalf("SaveOverrides: %v", err)
	}

	got, err := LoadOverrides(path)
	if err != nil {
		t.Fatalf("LoadOverrides: %v", err)
	}

	if !reflect.DeepEqual(got, ov) {
		t.Errorf("round-trip mismatch:\n  got  %v\n  want %v", got, ov)
	}
}

func TestLoadOverridesMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope.yaml")

	got, err := LoadOverrides(path)
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil empty Overrides, got nil")
	}
	if len(got) != 0 {
		t.Errorf("expected empty Overrides, got %v", got)
	}
}

func TestApplyFoldsHidesSubsumedBranches(t *testing.T) {
	slices := []model.Slice{
		{Name: "stack", Members: map[string]model.SliceMember{
			"web": {Repo: "web", Branch: "pay-105"}, // the tip (representative)
		}},
		{Name: "pay-103", Members: map[string]model.SliceMember{
			"web": {Repo: "web", Branch: "pay-103"},
		}},
		{Name: "pay-104", Members: map[string]model.SliceMember{
			"web": {Repo: "web", Branch: "pay-104"},
		}},
		{Name: "unrelated", Members: map[string]model.SliceMember{
			"web": {Repo: "web", Branch: "other"},
		}},
	}
	folded := Folded{"stack": {"web": {"pay-103", "pay-104"}}}

	got := ApplyFolds(slices, folded)

	names := make([]string, 0, len(got))
	for _, s := range got {
		names = append(names, s.Name)
	}
	want := []string{"stack", "unrelated"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("kept slices = %v; want %v (folded intermediates hidden, tip + unrelated kept)", names, want)
	}
}

func TestApplyFoldsRepoScoped(t *testing.T) {
	// A branch named "pay-103" in repo "api" is NOT folded by a web-repo fold.
	slices := []model.Slice{
		{Name: "api-103", Members: map[string]model.SliceMember{
			"api": {Repo: "api", Branch: "pay-103"},
		}},
	}
	got := ApplyFolds(slices, Folded{"stack": {"web": {"pay-103"}}})
	if len(got) != 1 {
		t.Errorf("api-103 dropped by a web-repo fold; folds must be repo-scoped")
	}
}

func TestSaveOverridesPreservesFolds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.yaml")
	if err := SaveConfig(path, Overrides{"stack": {"web": "pay-105"}}, Folded{"stack": {"web": {"pay-103", "pay-104"}}}); err != nil {
		t.Fatal(err)
	}
	// A grouping-only save must not wipe the folded section.
	if err := SaveOverrides(path, Overrides{"stack": {"web": "pay-105"}, "x": {"api": "feat"}}); err != nil {
		t.Fatal(err)
	}
	folded, err := LoadFolded(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(folded, Folded{"stack": {"web": {"pay-103", "pay-104"}}}) {
		t.Errorf("folds lost after SaveOverrides: %v", folded)
	}
}
