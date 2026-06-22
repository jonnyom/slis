package hooks_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jonnyom/slis/internal/hooks"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/notify"
)

func TestStatusForEvent(t *testing.T) {
	tests := []struct {
		event string
		want  model.SessionStatus
	}{
		{"Notification", model.SessWaitingInput},
		{"Stop", model.SessDone},
		{"SubagentStop", model.SessDone},
		{"Whatever", model.SessRunning},
		{"", model.SessRunning},
	}
	for _, tc := range tests {
		t.Run(tc.event, func(t *testing.T) {
			got := hooks.StatusForEvent(tc.event)
			if got != tc.want {
				t.Errorf("StatusForEvent(%q) = %v, want %v", tc.event, got, tc.want)
			}
		})
	}
}

func TestSliceForCwd(t *testing.T) {
	// Build a Slice with a member whose WorktreePath is a real temp dir.
	worktreeDir := t.TempDir()

	slices := []model.Slice{
		{
			Name: "my-slice",
			Members: map[string]model.SliceMember{
				"repo-a": {
					Repo:         "repo-a",
					Branch:       "feat/my-slice",
					WorktreePath: worktreeDir,
				},
			},
		},
	}

	t.Run("exact match", func(t *testing.T) {
		got := hooks.SliceForCwd(slices, worktreeDir)
		if got != "my-slice" {
			t.Errorf("SliceForCwd(exact) = %q, want %q", got, "my-slice")
		}
	})

	t.Run("subdir match", func(t *testing.T) {
		subdir := fmt.Sprintf("%s/pkg/foo", worktreeDir)
		got := hooks.SliceForCwd(slices, subdir)
		if got != "my-slice" {
			t.Errorf("SliceForCwd(subdir) = %q, want %q", got, "my-slice")
		}
	})

	t.Run("unrelated path", func(t *testing.T) {
		got := hooks.SliceForCwd(slices, "/some/other/path")
		if got != "" {
			t.Errorf("SliceForCwd(unrelated) = %q, want empty", got)
		}
	})
}

func TestHandleHookWritesStatus(t *testing.T) {
	worktreeDir := t.TempDir()
	eventsDir := t.TempDir()

	slices := []model.Slice{
		{
			Name: "my-slice",
			Members: map[string]model.SliceMember{
				"repo-a": {
					Repo:         "repo-a",
					Branch:       "feat/my-slice",
					WorktreePath: worktreeDir,
				},
			},
		},
	}

	t.Run("matching cwd writes status", func(t *testing.T) {
		body := fmt.Sprintf(`{"cwd":%q,"hook_event_name":"Notification"}`, worktreeDir)
		r := strings.NewReader(body)
		if err := hooks.HandleHook("Notification", r, slices, eventsDir, 1); err != nil {
			t.Fatalf("HandleHook: %v", err)
		}
		got := notify.ReadStatus(eventsDir, "my-slice")
		if got != model.SessWaitingInput {
			t.Errorf("ReadStatus = %v, want SessWaitingInput", got)
		}
	})

	t.Run("unmatched cwd is no-op", func(t *testing.T) {
		eventsDir2 := t.TempDir()
		body := `{"cwd":"/totally/unrelated/path","hook_event_name":"Notification"}`
		r := strings.NewReader(body)
		if err := hooks.HandleHook("Notification", r, slices, eventsDir2, 1); err != nil {
			t.Fatalf("HandleHook unmatched: %v", err)
		}
		// No files should have been written.
		all := notify.ReadAllStatuses(eventsDir2)
		if len(all) != 0 {
			t.Errorf("ReadAllStatuses = %v, want empty map", all)
		}
	})

	t.Run("empty body is no-op", func(t *testing.T) {
		eventsDir3 := t.TempDir()
		r := strings.NewReader("")
		if err := hooks.HandleHook("Stop", r, slices, eventsDir3, 1); err != nil {
			t.Fatalf("HandleHook empty: %v", err)
		}
	})

	t.Run("garbage body is no-op", func(t *testing.T) {
		eventsDir4 := t.TempDir()
		r := strings.NewReader("not json at all")
		if err := hooks.HandleHook("Stop", r, slices, eventsDir4, 1); err != nil {
			t.Fatalf("HandleHook garbage: %v", err)
		}
	})
}
