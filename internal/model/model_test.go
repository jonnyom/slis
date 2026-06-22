package model

import (
	"testing"
)

func TestSessionStatusString(t *testing.T) {
	tests := []struct {
		status SessionStatus
		want   string
	}{
		{SessNone, "none"},
		{SessRunning, "running"},
		{SessWaitingInput, "waiting-input"},
		{SessDone, "done"},
		{SessionStatus(99), "unknown"},
	}
	for _, tc := range tests {
		got := tc.status.String()
		if got != tc.want {
			t.Errorf("SessionStatus(%d).String() = %q, want %q", tc.status, got, tc.want)
		}
	}
}

func TestSliceRepos(t *testing.T) {
	s := Slice{
		Name: "my-slice",
		Base: "main",
		Members: map[string]SliceMember{
			"web": {Repo: "web", Branch: "feat/x", WorktreePath: "/tmp/web", TipSHA: "abc"},
			"api": {Repo: "api", Branch: "feat/x", WorktreePath: "/tmp/api", TipSHA: "def"},
			"ops": {Repo: "ops", Branch: "feat/x", WorktreePath: "/tmp/ops", TipSHA: "ghi"},
		},
	}
	repos := s.Repos()
	if len(repos) != 3 {
		t.Fatalf("Repos() returned %d entries, want 3", len(repos))
	}
	want := []string{"api", "ops", "web"}
	for i, r := range repos {
		if r != want[i] {
			t.Errorf("Repos()[%d] = %q, want %q", i, r, want[i])
		}
	}

	// Empty slice has zero-length Repos
	empty := Slice{}
	if got := empty.Repos(); len(got) != 0 {
		t.Errorf("empty Slice.Repos() returned %d entries, want 0", len(got))
	}
}
