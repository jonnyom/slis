package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/diff"
	"github.com/jonnyom/slis/internal/forge"
	"github.com/jonnyom/slis/internal/gt"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/proc"
)

func smokeModel() Model {
	m := New(config.Workspace{})
	m.loading = false
	m.slices = []model.Slice{
		{
			Name: "payroll-entity-brand-id",
			Members: map[string]model.SliceMember{
				"Web-App":         {Repo: "Web-App", Branch: "jonny/payroll-entity-brand-id-fe", WorktreePath: "/tmp/w"},
				"Node-Middleware": {Repo: "Node-Middleware", Branch: "jonny/payroll-entity-brand-id-mw", WorktreePath: "/tmp/n"},
				"nory":            {Repo: "nory", Branch: "jonny/payroll-id-entity-dedup-csv-sync", WorktreePath: "/tmp/y"},
			},
		},
		{Name: "wfm-4116-schedule-activity-export", Members: map[string]model.SliceMember{
			"Node-Middleware": {Repo: "Node-Middleware", Branch: "jonny/wfm-4116", WorktreePath: "/tmp/n2"},
		}},
	}
	m.sessionStatus = map[string]model.SessionStatus{
		"payroll-entity-brand-id":           model.SessWaitingInput,
		"wfm-4116-schedule-activity-export": model.SessRunning,
	}
	m.cards = map[string]sliceCard{
		"payroll-entity-brand-id":           {overview: "Scope payroll-ID dedup to employer boundaries", added: 279, deleted: 31, commits: 6, restack: 0, stackKnown: true},
		"wfm-4116-schedule-activity-export": {overview: "Schedule activity CSV export", added: 94, deleted: 0, commits: 2, restack: 2, stackKnown: true},
	}
	m.stacks = map[string]map[string]gt.State{
		"payroll-entity-brand-id": {
			"Web-App":         {"master": {Trunk: true}, "jonny/payroll-entity-brand-id-fe": {Parents: []gt.Parent{{Ref: "master"}}}},
			"Node-Middleware": {"master": {Trunk: true}, "jonny/payroll-entity-brand-id-mw": {Parents: []gt.Parent{{Ref: "master"}}, NeedsRestack: true}},
			"nory":            {"main": {Trunk: true}, "jonny/payroll-id-entity-dedup-csv-sync": {Parents: []gt.Parent{{Ref: "main"}}}},
		},
	}
	m.diffs = map[string][]diff.RepoDiff{
		"payroll-entity-brand-id": {
			{Repo: "Node-Middleware", Files: []diff.FileStat{{Path: "src/helpers.py", Added: 40, Deleted: 27}}, Patch: "@@ -1 +1 @@\n-old\n+new\n"},
			{Repo: "Web-App", Files: []diff.FileStat{{Path: "app.tsx", Added: 10, Deleted: 2}}, Patch: "@@ -1 +1 @@\n+x\n"},
			{Repo: "nory", Files: []diff.FileStat{{Path: "x.py", Added: 94, Deleted: 0}}, Patch: ""},
		},
	}
	m.prs = map[string]map[string]*forge.PR{
		"payroll-entity-brand-id": {
			"Web-App": {Number: 1240, Title: "feat: brand id", State: "OPEN", Checks: []forge.Check{{Name: "ci", State: forge.CheckPass}}, Comments: []forge.Comment{{Author: "a", Body: "ok"}}},
			"nory":    nil,
		},
	}
	m.procs = map[string][]proc.ProcInfo{
		"payroll-entity-brand-id": {{PID: 42, CPU: 142.3, MemMB: 512, Cmd: "node server.js"}},
	}
	m.summaries = map[string]string{"payroll-entity-brand-id": "## Web-App\n\n- feat: scope ids\n"}
	return m
}

// TestSmokeRenderNoPanic renders both views across a range of sizes and panels
// to catch width-math panics and obvious layout breakage.
func TestSmokeRenderNoPanic(t *testing.T) {
	sizes := [][2]int{{120, 40}, {80, 24}, {200, 60}, {61, 16}, {50, 10}}
	for _, sz := range sizes {
		m := smokeModel()
		next, _ := m.Update(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
		m = next.(Model)

		// Browser.
		_ = m.View()

		// Cockpit, every panel + summary.
		mc := next.(Model)
		mc.view = viewCockpit
		mc.resizeViewport()
		for p := panel(0); p < panelCount; p++ {
			mc.panel = p
			mc.refreshRight()
			_ = mc.View()
		}
		mc.right = rightSummary
		mc.refreshRight()
		_ = mc.View()
	}
}

// TestSmokeDump prints a 120x40 browser and cockpit for manual eyeballing
// (run with: go test ./internal/tui/ -run TestSmokeDump -v).
func TestSmokeDump(t *testing.T) {
	m := smokeModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	t.Logf("\n=== BROWSER ===\n%s", m.View())

	mc := next.(Model)
	mc.view = viewCockpit
	mc.resizeViewport()
	mc.refreshRight()
	t.Logf("\n=== COCKPIT (Stack focused) ===\n%s", mc.View())
}
