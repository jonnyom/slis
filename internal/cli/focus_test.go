package cli

import (
	"os/exec"
	"testing"

	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/tmuxctl"
)

func TestDecideFocus(t *testing.T) {
	target := tmuxctl.SessionName("alpha")

	t.Run("no attached clients → attach hint", func(t *testing.T) {
		action, _ := decideFocus(nil, target)
		if action != focusAttachHint {
			t.Errorf("action = %v, want focusAttachHint", action)
		}
	})

	t.Run("chosen client already on target → no-op", func(t *testing.T) {
		clients := []tmuxctl.Client{
			{TTY: "/dev/ttys001", Session: target, LastActivity: 500},
			{TTY: "/dev/ttys002", Session: "other", LastActivity: 100},
		}
		action, c := decideFocus(clients, target)
		if action != focusAlready {
			t.Errorf("action = %v, want focusAlready", action)
		}
		if c.TTY != "/dev/ttys001" {
			t.Errorf("client = %q, want the most-recent one on target", c.TTY)
		}
	})

	t.Run("most-recent client elsewhere → switch it", func(t *testing.T) {
		clients := []tmuxctl.Client{
			{TTY: "/dev/ttys001", Session: target, LastActivity: 100},
			{TTY: "/dev/ttys002", Session: "other", LastActivity: 900},
		}
		action, c := decideFocus(clients, target)
		if action != focusSwitch {
			t.Errorf("action = %v, want focusSwitch", action)
		}
		if c.TTY != "/dev/ttys002" {
			t.Errorf("client = %q, want the most-recently-active client /dev/ttys002", c.TTY)
		}
	})
}

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

// TestFocusDetachedSessionNeverAlready is a live tmux test: a freshly created,
// detached session has no client viewing it, so decideFocus over the real client
// list must never report focusAlready (it prints an attach hint when nothing is
// attached, or switches an existing client — but never no-ops). This holds
// whether or not the test itself runs inside a tmux client.
func TestFocusDetachedSessionNeverAlready(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not found on PATH")
	}

	const slice = "slistest-focus"
	_ = tmuxctl.KillSession(slice)
	t.Cleanup(func() { _ = tmuxctl.KillSession(slice) })

	members := []model.SliceMember{{Repo: "alpha", WorktreePath: t.TempDir()}}
	if err := tmuxctl.EnsureSession(slice, members, tmuxctl.SessionOpts{}); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	clients, err := tmuxctl.ListClients()
	if err != nil {
		t.Fatalf("ListClients: %v", err)
	}
	if action, _ := decideFocus(clients, tmuxctl.SessionName(slice)); action == focusAlready {
		t.Error("decideFocus returned focusAlready for a detached session with no client on it")
	}
}
