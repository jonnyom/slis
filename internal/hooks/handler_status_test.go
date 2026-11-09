package hooks_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/hooks"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/notify"
)

type hookSlice struct {
	t         *testing.T
	worktree  string
	eventsDir string
	slices    []model.Slice
}

func newHookSlice(t *testing.T) hookSlice {
	worktree := t.TempDir()
	return hookSlice{
		t:         t,
		worktree:  worktree,
		eventsDir: t.TempDir(),
		slices: []model.Slice{{
			Name: "feature",
			Members: map[string]model.SliceMember{
				"api": {Repo: "api", Branch: "feature", WorktreePath: worktree},
			},
		}},
	}
}

func (h hookSlice) fire(event, session, extraJSON string) {
	h.t.Helper()
	body := fmt.Sprintf(`{"cwd":%q,"session_id":%q%s}`, h.worktree, session, extraJSON)
	if err := hooks.HandleHook(event, strings.NewReader(body), h.slices, h.eventsDir, config.Notify{}, 1); err != nil {
		h.t.Fatalf("HandleHook(%s): %v", event, err)
	}
}

func (h hookSlice) summary() notify.SliceSummary {
	return notify.ReadAllStatuses(h.eventsDir)["feature"]
}

func (h hookSlice) expect(status model.SessionStatus, subagents int) {
	h.t.Helper()
	want := notify.SliceSummary{Status: status, Subagents: subagents}
	if status == model.SessNone {
		want = notify.SliceSummary{}
	}
	if got := h.summary(); got != want {
		h.t.Fatalf("summary = %+v, want %+v", got, want)
	}
}

const twoBackgroundSubagents = `,"background_tasks":[` +
	`{"id":"a1","type":"subagent","status":"running"},` +
	`{"id":"a2","type":"subagent","status":"running"},` +
	`{"id":"sh","type":"shell","status":"running"}]`

func TestStopWithBackgroundSubagentsShowsSubagentsWorking(t *testing.T) {
	h := newHookSlice(t)
	h.fire("UserPromptSubmit", "main", "")
	h.fire("Stop", "main", twoBackgroundSubagents)
	h.expect(model.SessSubagents, 2)
}

func TestStopWithoutBackgroundWorkIsDone(t *testing.T) {
	h := newHookSlice(t)
	h.fire("UserPromptSubmit", "main", "")
	h.fire("Stop", "main", `,"background_tasks":[]`)
	h.expect(model.SessDone, 0)
}

func TestSubagentStopWhileMainWorksKeepsRunning(t *testing.T) {
	h := newHookSlice(t)
	h.fire("UserPromptSubmit", "main", "")
	h.fire("SubagentStop", "main", `,"agent_id":"a1","background_tasks":[]`)
	h.expect(model.SessRunning, 0)
}

func TestSubagentStopCountsDownThenFinishes(t *testing.T) {
	h := newHookSlice(t)
	h.fire("Stop", "main", twoBackgroundSubagents)
	h.fire("SubagentStop", "main", `,"agent_id":"a1","background_tasks":[`+
		`{"id":"a1","type":"subagent","status":"completed"},`+
		`{"id":"a2","type":"subagent","status":"running"}]`)
	h.expect(model.SessSubagents, 1)
	h.fire("SubagentStop", "main", `,"agent_id":"a2","background_tasks":[]`)
	h.expect(model.SessDone, 0)
}

func TestSubagentToolUseDoesNotMarkIdleMainThreadRunning(t *testing.T) {
	h := newHookSlice(t)
	h.fire("Stop", "main", twoBackgroundSubagents)
	h.fire("PostToolUse", "main", `,"agent_id":"a1"`)
	h.expect(model.SessSubagents, 2)
}

func TestMainThreadToolUseAfterPermissionPromptIsRunning(t *testing.T) {
	h := newHookSlice(t)
	h.fire("Notification", "main", `,"notification_type":"permission_prompt"`)
	h.expect(model.SessWaitingInput, 0)
	h.fire("PostToolUse", "main", "")
	h.expect(model.SessRunning, 0)
}

func TestNotificationTypes(t *testing.T) {
	tests := []struct {
		notificationType string
		want             model.SessionStatus
	}{
		{"permission_prompt", model.SessWaitingInput},
		{"elicitation_dialog", model.SessWaitingInput},
		{"agent_needs_input", model.SessWaitingInput},
		{"idle_prompt", model.SessDone},
		{"auth_success", model.SessRunning},
	}
	for _, tc := range tests {
		t.Run(tc.notificationType, func(t *testing.T) {
			h := newHookSlice(t)
			h.fire("UserPromptSubmit", "main", "")
			h.fire("Notification", "main", fmt.Sprintf(`,"notification_type":%q`, tc.notificationType))
			h.expect(tc.want, 0)
		})
	}
}

func TestSessionsInOneSliceDoNotOverwriteEachOther(t *testing.T) {
	h := newHookSlice(t)
	h.fire("Notification", "first", `,"notification_type":"permission_prompt"`)
	h.fire("Stop", "second", `,"background_tasks":[]`)
	h.expect(model.SessWaitingInput, 0)
}

func TestSessionEndRemovesOnlyThatSession(t *testing.T) {
	h := newHookSlice(t)
	h.fire("Notification", "first", `,"notification_type":"permission_prompt"`)
	h.fire("UserPromptSubmit", "second", "")
	h.fire("SessionEnd", "first", "")
	h.expect(model.SessRunning, 0)
	h.fire("SessionEnd", "second", "")
	h.expect(model.SessNone, 0)
}
