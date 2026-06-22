package cli

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/swap"
	"github.com/jonnyom/slis/testutil"
)

// makeTestWorkspace creates 3 repos with worktrees:
//
//	web  → jonny/checkout
//	api  → jonny/checkout
//	ops  → jonny/other
//
// and returns a Workspace configured to strip the "jonny/" prefix.
func makeTestWorkspace(t *testing.T) config.Workspace {
	t.Helper()
	web := testutil.NewRepo(t)
	api := testutil.NewRepo(t)
	ops := testutil.NewRepo(t)

	base := t.TempDir()
	testutil.AddWorktree(t, web, "jonny/checkout", filepath.Join(base, "web-checkout"))
	testutil.AddWorktree(t, api, "jonny/checkout", filepath.Join(base, "api-checkout"))
	testutil.AddWorktree(t, ops, "jonny/other", filepath.Join(base, "ops-other"))

	return config.Workspace{
		Root: base,
		Repos: map[string]config.Repo{
			"web": {Primary: web, DefaultBranch: "main"},
			"api": {Primary: api, DefaultBranch: "main"},
			"ops": {Primary: ops, DefaultBranch: "main"},
		},
		Grouping: config.Grouping{
			Strategy:    "branch-name",
			StripPrefix: "jonny/",
		},
	}
}

func TestListSlices(t *testing.T) {
	ws := makeTestWorkspace(t)

	// Use non-existent paths so there are no overrides, no active journal.
	tmp := t.TempDir()
	ovPath := filepath.Join(tmp, "ov.yaml")
	jPath := filepath.Join(tmp, "none.json")

	dtos, err := listSlices(ws, ovPath, jPath)
	if err != nil {
		t.Fatalf("listSlices: %v", err)
	}

	if len(dtos) != 2 {
		t.Fatalf("want 2 DTOs, got %d: %v", len(dtos), dtos)
	}

	// DTOs are sorted by Name, so checkout < other.
	checkout := dtos[0]
	other := dtos[1]

	if checkout.Name != "checkout" {
		t.Errorf("dtos[0].Name = %q, want %q", checkout.Name, "checkout")
	}
	if other.Name != "other" {
		t.Errorf("dtos[1].Name = %q, want %q", other.Name, "other")
	}

	// checkout should have web and api members.
	if len(checkout.Members) != 2 {
		t.Errorf("checkout Members count = %d, want 2", len(checkout.Members))
	}
	repoNames := make(map[string]bool)
	for _, m := range checkout.Members {
		repoNames[m.Repo] = true
	}
	if !repoNames["web"] {
		t.Error("checkout missing 'web' member")
	}
	if !repoNames["api"] {
		t.Error("checkout missing 'api' member")
	}

	// other should have only ops.
	if len(other.Members) != 1 {
		t.Errorf("other Members count = %d, want 1", len(other.Members))
	}
	if other.Members[0].Repo != "ops" {
		t.Errorf("other member Repo = %q, want 'ops'", other.Members[0].Repo)
	}

	// None should be active since no journal exists.
	for _, dto := range dtos {
		if dto.Active {
			t.Errorf("slice %q should not be active (no journal)", dto.Name)
		}
	}
}

func TestListSlicesMarksActive(t *testing.T) {
	ws := makeTestWorkspace(t)

	tmp := t.TempDir()
	ovPath := filepath.Join(tmp, "ov.yaml")
	jPath := filepath.Join(tmp, "active.json")

	// Write a journal marking "checkout" as active.
	if err := swap.Save(jPath, &swap.Journal{Slice: "checkout", Repos: nil}); err != nil {
		t.Fatalf("swap.Save: %v", err)
	}

	dtos, err := listSlices(ws, ovPath, jPath)
	if err != nil {
		t.Fatalf("listSlices: %v", err)
	}

	var foundCheckout, foundOther bool
	for _, dto := range dtos {
		if dto.Name == "checkout" {
			foundCheckout = true
			if !dto.Active {
				t.Error("checkout should be Active=true")
			}
		}
		if dto.Name == "other" {
			foundOther = true
			if dto.Active {
				t.Error("other should be Active=false")
			}
		}
	}
	if !foundCheckout {
		t.Error("did not find 'checkout' DTO")
	}
	if !foundOther {
		t.Error("did not find 'other' DTO")
	}
}

func TestListSlicesJSON(t *testing.T) {
	ws := makeTestWorkspace(t)

	tmp := t.TempDir()
	ovPath := filepath.Join(tmp, "ov.yaml")
	jPath := filepath.Join(tmp, "none.json")

	dtos, err := listSlices(ws, ovPath, jPath)
	if err != nil {
		t.Fatalf("listSlices: %v", err)
	}

	data, err := json.Marshal(dtos)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	// Verify valid JSON containing "checkout".
	var out []SliceDTO
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("json.Unmarshal roundtrip: %v", err)
	}

	found := false
	for _, dto := range out {
		if dto.Name == "checkout" {
			found = true
		}
	}
	if !found {
		t.Error("JSON output does not contain a 'checkout' slice")
	}
}
