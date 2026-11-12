package cli

import (
	"reflect"
	"testing"
	"time"

	"github.com/jonnyom/slis/internal/notify"
	"github.com/jonnyom/slis/internal/proc"
	sessionmanager "github.com/jonnyom/slis/internal/session"
)

func TestIdleClaudeTabsSelectsOnlyClaudeTabsOfSlicesQuietPastTheThreshold(t *testing.T) {
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	twoDaysAgo := now.Add(-48 * time.Hour).UnixNano()
	threeHoursAgo := now.Add(-3 * time.Hour).UnixNano()
	thirtyMinutesAgo := now.Add(-30 * time.Minute).UnixNano()
	groups := []sessionmanager.Group{
		{ID: "quiet", Tabs: []sessionmanager.Tab{{ID: "agent"}, {ID: "shell"}}},
		{ID: "recent", Tabs: []sessionmanager.Tab{{ID: "agent"}}},
		{ID: "working", Tabs: []sessionmanager.Tab{{ID: "agent"}}},
		{ID: "crashed", Tabs: []sessionmanager.Tab{{ID: "agent"}}},
		{ID: "no-hooks", Tabs: []sessionmanager.Tab{{ID: "agent"}}},
	}
	claudeTree := []proc.ProcInfo{{PID: 1, Cmd: "/bin/zsh"}, {PID: 2, PPID: 1, Cmd: "claude --resume abc", MemMB: 300}, {PID: 3, PPID: 2, Cmd: "node tsserver.js", MemMB: 2000}}
	runtimes := []sessionmanager.TabRuntime{
		{GroupID: "quiet", TabID: "agent", Processes: claudeTree},
		{GroupID: "quiet", TabID: "shell", Processes: []proc.ProcInfo{{PID: 4, Cmd: "npm run dev", MemMB: 500}}},
		{GroupID: "recent", TabID: "agent", Processes: claudeTree},
		{GroupID: "working", TabID: "agent", Processes: claudeTree},
		{GroupID: "crashed", TabID: "agent", Processes: claudeTree},
		{GroupID: "no-hooks", TabID: "agent", Processes: claudeTree},
	}
	recordsBySlice := map[string][]notify.Status{
		"quiet":   {{Status: "done", TimeNS: twoDaysAgo, SessionID: "session-1"}, {Status: "waiting-input", TimeNS: threeHoursAgo, SessionID: "session-1"}},
		"recent":  {{Status: "done", TimeNS: twoDaysAgo, SessionID: "old"}, {Status: "waiting-input", TimeNS: thirtyMinutesAgo, SessionID: "new"}},
		"working": {{Status: "running", TimeNS: threeHoursAgo, SessionID: "busy"}},
		"crashed": {{Status: "running", TimeNS: twoDaysAgo, SessionID: "gone"}},
	}

	candidates := idleClaudeTabs(groups, runtimes, recordsBySlice, now, time.Hour)

	want := []reapCandidate{
		{Slice: "quiet", Tab: "agent", IdleHours: 3, MemoryMB: 2300, ResumeSessionIDs: []string{"session-1"}},
		{Slice: "crashed", Tab: "agent", IdleHours: 48, MemoryMB: 2300, ResumeSessionIDs: []string{"gone"}},
	}
	if !reflect.DeepEqual(candidates, want) {
		t.Fatalf("candidates = %#v, want %#v", candidates, want)
	}
}
