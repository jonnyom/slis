package cli

import (
	"fmt"
	"testing"

	"github.com/jonnyom/slis/internal/model"
)

func TestMembersOfSliceSorted(t *testing.T) {
	sl := model.Slice{
		Name: "s",
		Members: map[string]model.SliceMember{
			"zeta":  {Repo: "zeta", WorktreePath: "/z"},
			"alpha": {Repo: "alpha", WorktreePath: "/a"},
		},
	}
	got := membersOfSlice(sl)
	if len(got) != 2 || got[0].Repo != "alpha" || got[1].Repo != "zeta" {
		t.Errorf("membersOfSlice = %+v, want [alpha, zeta]", got)
	}
}

func TestDetachedSessionAttachArgvForGhostty(t *testing.T) {
	name, args, ok := detachedSlisAttachArgv("ghostty", "alpha", "agent")
	if !ok || name != "open" {
		t.Fatalf("detachedSlisAttachArgv = %q, %v, %v", name, args, ok)
	}
	want := []string{"-na", "Ghostty.app", "--args", "-e", "slis", "session", "attach", "alpha", "agent"}
	if fmt.Sprint(args) != fmt.Sprint(want) {
		t.Fatalf("args = %v, want %v", args, want)
	}
}

func TestDetachedSessionAttachArgvRejectsUnknownTerminal(t *testing.T) {
	if _, _, ok := detachedSlisAttachArgv("unknown", "alpha", "agent"); ok {
		t.Fatal("unknown terminal unexpectedly supported")
	}
}
