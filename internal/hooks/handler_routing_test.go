package hooks_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/hooks"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/notify"
)

func mkdirs(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func sliceWith(name string, worktrees ...string) model.Slice {
	members := map[string]model.SliceMember{}
	for _, wt := range worktrees {
		repo := filepath.Base(wt)
		members[repo] = model.SliceMember{Repo: repo, Branch: name, WorktreePath: wt}
	}
	return model.Slice{Name: name, Members: members}
}

func TestSliceForCwdMatchesSliceRoot(t *testing.T) {
	worktrees := t.TempDir()
	root := filepath.Join(worktrees, "feature")
	api, web := filepath.Join(root, "api"), filepath.Join(root, "web")
	other := filepath.Join(worktrees, "other", "api")
	mkdirs(t, api, web, other)
	slices := []model.Slice{sliceWith("feature", api, web), sliceWith("other", other)}

	if got := hooks.SliceForCwd(slices, root); got != "feature" {
		t.Errorf("SliceForCwd(slice root) = %q, want feature", got)
	}
	if got := hooks.SliceForCwd(slices, filepath.Join(root, "notes")); got != "feature" {
		t.Errorf("SliceForCwd(dir under slice root) = %q, want feature", got)
	}
	if got := hooks.SliceForCwd(slices, worktrees); got != "" {
		t.Errorf("SliceForCwd(worktrees dir) = %q, want empty", got)
	}
}

func TestSliceForCwdIgnoresParentNotNamedAfterSlice(t *testing.T) {
	code := t.TempDir()
	feature := filepath.Join(code, "api-feature")
	mkdirs(t, feature)
	slices := []model.Slice{sliceWith("feature", feature)}

	if got := hooks.SliceForCwd(slices, code); got != "" {
		t.Errorf("SliceForCwd(adopted worktree's parent) = %q, want empty", got)
	}
}

func TestSliceForCwdIgnoresRootHoldingAnotherSlice(t *testing.T) {
	root := filepath.Join(t.TempDir(), "feature")
	api, other := filepath.Join(root, "api"), filepath.Join(root, "web", "other")
	mkdirs(t, api, other)
	slices := []model.Slice{sliceWith("feature", api), sliceWith("other", other)}

	if got := hooks.SliceForCwd(slices, root); got != "" {
		t.Errorf("SliceForCwd(root shared with other) = %q, want empty", got)
	}
}

func fireAt(t *testing.T, cwd, session, eventsDir string, slices []model.Slice, event string, timeNS int64) {
	t.Helper()
	body := fmt.Sprintf(`{"cwd":%q,"session_id":%q}`, cwd, session)
	if err := hooks.HandleHook(event, strings.NewReader(body), slices, eventsDir, config.Notify{}, timeNS); err != nil {
		t.Fatalf("HandleHook(%s): %v", event, err)
	}
}

func TestHandleHookFollowsSessionOutsideItsSlice(t *testing.T) {
	worktree := t.TempDir()
	eventsDir := t.TempDir()
	slices := []model.Slice{sliceWith("feature", worktree)}

	fireAt(t, worktree, "s1", eventsDir, slices, "UserPromptSubmit", 1)
	fireAt(t, t.TempDir(), "s1", eventsDir, slices, "Stop", 2)

	if got := notify.ReadAllStatuses(eventsDir)["feature"].Status; got != model.SessDone {
		t.Errorf("status = %v, want done", got)
	}
	if _, ok := notify.ReadUnmatchedHook(eventsDir); ok {
		t.Error("recorded an unmatched hook for a known session")
	}
}

func TestHandleHookRecordsUnmatchedEvent(t *testing.T) {
	eventsDir := t.TempDir()
	slices := []model.Slice{sliceWith("feature", t.TempDir())}
	stray := filepath.Join(t.TempDir(), ".slis", "worktrees", "removed-slice")
	mkdirs(t, stray)

	fireAt(t, stray, "s1", eventsDir, slices, "Stop", 7)

	if got := notify.ReadAllStatuses(eventsDir); len(got) != 0 {
		t.Errorf("statuses = %+v, want none", got)
	}
	want := notify.UnmatchedHook{Event: "Stop", TimeNS: 7, SessionID: "s1", Cwd: stray}
	if got, ok := notify.ReadUnmatchedHook(eventsDir); !ok || got != want {
		t.Errorf("unmatched = %+v (ok=%v), want %+v", got, ok, want)
	}
}

func TestHandleHookIgnoresSessionsOutsideSlisWorktrees(t *testing.T) {
	eventsDir := t.TempDir()
	slices := []model.Slice{sliceWith("feature", t.TempDir())}

	fireAt(t, t.TempDir(), "s1", eventsDir, slices, "Stop", 7)

	if got, ok := notify.ReadUnmatchedHook(eventsDir); ok {
		t.Errorf("recorded unmatched hook %+v for a session outside slis worktrees", got)
	}
}
