package tui

import (
	"os/exec"
	"testing"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/model"
)

func TestWorktreePaths(t *testing.T) {
	sl := model.Slice{
		Name: "s",
		Members: map[string]model.SliceMember{
			"z-repo": {Repo: "z-repo", WorktreePath: "/wt/s/z"},
			"a-repo": {Repo: "a-repo", WorktreePath: "/wt/s/a"},
			"empty":  {Repo: "empty", WorktreePath: ""}, // skipped
		},
	}
	got := worktreePaths(sl)
	// Sorted repo order: a-repo, empty (skipped), z-repo.
	if len(got) != 2 || got[0] != "/wt/s/a" || got[1] != "/wt/s/z" {
		t.Fatalf("worktreePaths = %v, want [/wt/s/a /wt/s/z]", got)
	}
}

func TestOpenInEditorConfigured(t *testing.T) {
	// Use a binary guaranteed on PATH so editor.Resolve succeeds without a real
	// editor; the dispatch should return a Cmd and NOT raise the picker.
	bin := "cat"
	if _, err := exec.LookPath(bin); err != nil {
		t.Skip("cat not on PATH")
	}
	m := New(config.Workspace{})
	m.ws.Sessions.Editor = bin
	sl := model.Slice{Name: "s", Members: map[string]model.SliceMember{
		"r": {Repo: "r", WorktreePath: "/wt/s/r"},
	}}
	cmd := m.openInEditor(editorReq{slice: sl})
	if cmd == nil {
		t.Error("openInEditor with a configured editor returned nil cmd")
	}
	if m.showEditorPicker {
		t.Error("picker raised even though an editor is configured")
	}
}
