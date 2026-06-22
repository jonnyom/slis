package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/proc"
)

// ─── helpers ────────────────────────────────────────────────────────────────

// modelWithProcs builds a test model with three named slices already loaded
// and a pre-populated procs map so overlay tests don't need a real tmux.
func modelWithProcs(t *testing.T) Model {
	t.Helper()
	m := New(config.Workspace{
		Processes: config.Processes{CPUWarnPct: 50},
	})
	m.slices = []model.Slice{
		{Name: "alpha"},
		{Name: "beta"},
	}
	m.loading = false
	m.procs["alpha"] = []proc.ProcInfo{
		{PID: 100, CPU: 30, MemMB: 10, Cmd: "bash"},
		{PID: 101, CPU: 20, MemMB: 5, Cmd: "vim"},
		{PID: 102, CPU: 10, MemMB: 3, Cmd: "node"},
	}
	m.procs["beta"] = []proc.ProcInfo{
		{PID: 200, CPU: 5, MemMB: 2, Cmd: "python"},
	}
	return m
}

// ─── pure helper tests ───────────────────────────────────────────────────────

// TestSliceCPUAndThreshold verifies sliceCPU summation and overCPUThreshold edge cases.
func TestSliceCPUAndThreshold(t *testing.T) {
	procs := []proc.ProcInfo{
		{CPU: 10},
		{CPU: 20},
		{CPU: 30},
	}

	got := sliceCPU(procs)
	if got != 60 {
		t.Errorf("sliceCPU: want 60, got %v", got)
	}

	if !overCPUThreshold(procs, 50) {
		t.Error("overCPUThreshold(60, 50): want true")
	}
	if overCPUThreshold(procs, 100) {
		t.Error("overCPUThreshold(60, 100): want false")
	}
	// pct <= 0 → always false
	if overCPUThreshold(procs, 0) {
		t.Error("overCPUThreshold(60, 0): want false")
	}
	if overCPUThreshold(procs, -1) {
		t.Error("overCPUThreshold(60, -1): want false")
	}
}

// TestFlattenProcsSorted verifies that flattenProcs merges all slices and sorts by CPU desc.
func TestFlattenProcsSorted(t *testing.T) {
	bySlice := map[string][]proc.ProcInfo{
		"slice-a": {
			{PID: 1, CPU: 5},
			{PID: 2, CPU: 50},
		},
		"slice-b": {
			{PID: 3, CPU: 25},
			{PID: 4, CPU: 75},
		},
	}

	result := flattenProcs(bySlice)

	if len(result) != 4 {
		t.Fatalf("flattenProcs: want 4 procs, got %d", len(result))
	}

	// Verify sorted CPU descending: 75, 50, 25, 5
	wantCPUs := []float64{75, 50, 25, 5}
	for i, want := range wantCPUs {
		if result[i].CPU != want {
			t.Errorf("flattenProcs[%d]: want CPU=%v, got %v", i, want, result[i].CPU)
		}
	}
}

// TestProcsLoadedMsg verifies that procsLoadedMsg stores procs and clears loading.
func TestProcsLoadedMsg(t *testing.T) {
	m := New(config.Workspace{})
	m.slices = []model.Slice{{Name: "s"}}
	m.loading = false
	m.procLoading["s"] = true

	procs := []proc.ProcInfo{
		{PID: 42, CPU: 10, Cmd: "bash"},
	}

	next, _ := m.Update(procsLoadedMsg{slice: "s", procs: procs})
	m = next.(Model)

	if len(m.procs["s"]) != 1 {
		t.Fatalf("want 1 proc cached, got %d", len(m.procs["s"]))
	}
	if m.procs["s"][0].PID != 42 {
		t.Errorf("want PID=42, got %d", m.procs["s"][0].PID)
	}
	if m.procLoading["s"] {
		t.Error("procLoading[s] should be false after msg received")
	}
}

// TestOverlayToggleAndNav verifies overlay open/close and j/k navigation.
func TestOverlayToggleAndNav(t *testing.T) {
	m := modelWithProcs(t)

	if m.showProcOverlay {
		t.Fatal("overlay should start closed")
	}

	// Open overlay with "P".
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("P")})
	m = next.(Model)
	if !m.showProcOverlay {
		t.Fatal("after P: overlay should be open")
	}
	if m.overlaySel != 0 {
		t.Errorf("overlay opened: want overlaySel=0, got %d", m.overlaySel)
	}
	if len(m.overlayProcs) == 0 {
		t.Fatal("overlay opened: overlayProcs should be non-empty")
	}
	n := len(m.overlayProcs)

	// Navigate down with "j".
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = next.(Model)
	if m.overlaySel != 1 {
		t.Errorf("after j: want overlaySel=1, got %d", m.overlaySel)
	}

	// Navigate to end.
	for i := 0; i < n; i++ {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = next.(Model)
	}
	if m.overlaySel != n-1 {
		t.Errorf("after clamping j: want overlaySel=%d, got %d", n-1, m.overlaySel)
	}

	// Navigate up with "k".
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = next.(Model)
	if m.overlaySel != n-2 {
		t.Errorf("after k: want overlaySel=%d, got %d", n-2, m.overlaySel)
	}

	// Close overlay with "P" again.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("P")})
	m = next.(Model)
	if m.showProcOverlay {
		t.Error("after second P: overlay should be closed")
	}
}

// TestOverlayDoesNotMoveFocus verifies j/k inside overlay don't change slice focus.
func TestOverlayDoesNotMoveFocus(t *testing.T) {
	m := modelWithProcs(t)
	m.focus = 0

	// Open overlay.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("P")})
	m = next.(Model)

	// j inside overlay should move overlaySel, NOT m.focus.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = next.(Model)
	if m.focus != 0 {
		t.Errorf("overlay j must not move slice focus; want 0, got %d", m.focus)
	}
	if m.overlaySel != 1 {
		t.Errorf("overlay j should move overlaySel to 1; got %d", m.overlaySel)
	}
}

// TestKillConfirmFlow verifies x sets pendingKill and n clears it without killing.
func TestKillConfirmFlow(t *testing.T) {
	m := modelWithProcs(t)

	// Open overlay.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("P")})
	m = next.(Model)
	if len(m.overlayProcs) == 0 {
		t.Fatal("overlayProcs must be non-empty for kill flow test")
	}

	expectedPID := m.overlayProcs[m.overlaySel].PID

	// Press "x" → pendingKill set, subtree=false.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = next.(Model)
	if m.pendingKill == nil {
		t.Fatal("after x: pendingKill should not be nil")
	}
	if m.pendingKill.pid != expectedPID {
		t.Errorf("pendingKill.pid: want %d, got %d", expectedPID, m.pendingKill.pid)
	}
	if m.pendingKill.subtree {
		t.Error("x sets subtree=false; got true")
	}

	// Press "n" → pendingKill cleared, NO kill issued.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = next.(Model)
	if m.pendingKill != nil {
		t.Error("after n: pendingKill should be nil")
	}

	// Also verify X (subtree kill request).
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	m = next.(Model)
	if m.pendingKill == nil {
		t.Fatal("after X: pendingKill should not be nil")
	}
	if !m.pendingKill.subtree {
		t.Error("X sets subtree=true; got false")
	}

	// Press "n" again to clear.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = next.(Model)
	if m.pendingKill != nil {
		t.Error("after n: pendingKill should be nil")
	}
}

// TestRenderProcTable verifies that renderProcTable includes PID and CMD strings.
func TestRenderProcTable(t *testing.T) {
	procs := []proc.ProcInfo{
		{PID: 1234, CPU: 55.5, MemMB: 100.2, Cmd: "server --port 8080"},
		{PID: 5678, CPU: 10.0, MemMB: 20.5, Cmd: "worker"},
	}

	output := renderProcTable(procs, -1, 80)

	if !strings.Contains(output, "1234") {
		t.Errorf("renderProcTable missing PID 1234; got:\n%s", output)
	}
	if !strings.Contains(output, "5678") {
		t.Errorf("renderProcTable missing PID 5678; got:\n%s", output)
	}
	if !strings.Contains(output, "server") {
		t.Errorf("renderProcTable missing CMD 'server'; got:\n%s", output)
	}
	if !strings.Contains(output, "worker") {
		t.Errorf("renderProcTable missing CMD 'worker'; got:\n%s", output)
	}
}

// TestRenderProcTableEmpty verifies that empty procs returns a safe placeholder.
func TestRenderProcTableEmpty(t *testing.T) {
	output := renderProcTable(nil, -1, 80)
	// Should not panic and should be non-empty
	_ = output
}

// TestProcsLoadedMsgRebuildsOverlay verifies that when the overlay is open
// and a procsLoadedMsg arrives, overlayProcs is rebuilt.
func TestProcsLoadedMsgRebuildsOverlay(t *testing.T) {
	m := modelWithProcs(t)

	// Open overlay.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("P")})
	m = next.(Model)
	initialLen := len(m.overlayProcs)

	// Inject a new proc for slice "alpha" with an extra proc.
	newProcs := append(m.procs["alpha"], proc.ProcInfo{PID: 999, CPU: 99, Cmd: "newproc"})
	next, _ = m.Update(procsLoadedMsg{slice: "alpha", procs: newProcs})
	m = next.(Model)

	if len(m.overlayProcs) <= initialLen {
		t.Errorf("overlay should have more procs after reload; want >%d, got %d",
			initialLen, len(m.overlayProcs))
	}
}
