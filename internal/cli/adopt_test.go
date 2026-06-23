package cli

import "testing"

func TestBuildAdoptCandidates(t *testing.T) {
	perRepo := map[string][]string{
		"web": {"main", "jonny/wfm-1", "jonny/wfm-2"},
		"api": {"main", "jonny/wfm-1"}, // shares wfm-1 with web
		"db":  {"master", "jonny/wfm-3-managed"},
	}
	trunks := map[string]string{"web": "main", "api": "main", "db": "master"}
	managed := map[string]bool{"jonny/wfm-3-managed": true} // already a slice

	got := buildAdoptCandidates("jonny/", perRepo, trunks, managed)

	// Expect wfm-1 (web+api) and wfm-2 (web); trunk + managed excluded.
	if len(got) != 2 {
		t.Fatalf("want 2 candidates, got %d: %+v", len(got), got)
	}
	if got[0].Slice != "wfm-1" {
		t.Errorf("first slice = %q, want wfm-1", got[0].Slice)
	}
	if got[0].Branch != "jonny/wfm-1" {
		t.Errorf("first branch = %q, want jonny/wfm-1", got[0].Branch)
	}
	if len(got[0].Repos) != 2 || got[0].Repos[0] != "api" || got[0].Repos[1] != "web" {
		t.Errorf("wfm-1 repos = %v, want [api web]", got[0].Repos)
	}
	if got[1].Slice != "wfm-2" {
		t.Errorf("second slice = %q, want wfm-2", got[1].Slice)
	}
}

func TestIsTrunkBranch(t *testing.T) {
	if !isTrunkBranch("main", "develop") {
		t.Error("main should be trunk by convention")
	}
	if !isTrunkBranch("develop", "develop") {
		t.Error("configured default should be trunk")
	}
	if isTrunkBranch("jonny/wfm-1", "main") {
		t.Error("feature branch should not be trunk")
	}
}
