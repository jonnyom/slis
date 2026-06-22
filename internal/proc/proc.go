// Package proc provides process tree sampling and termination utilities.
// It walks descendant trees of given pane PIDs (as returned by
// internal/tmuxctl.PanePIDs) to report CPU/memory usage and kill runaway
// processes.
package proc

import (
	"sort"
	"syscall"

	goproc "github.com/shirou/gopsutil/v4/process"
)

// ProcInfo holds snapshot data for a single process.
type ProcInfo struct {
	PID   int
	PPID  int
	CPU   float64 // cumulative CPU percent since process start (non-blocking)
	MemMB float64 // RSS in MiB
	Cmd   string  // command line (may be truncated)
}

// SliceProcs returns all processes in the descendant trees of the given pane
// PIDs (including the pane PIDs themselves), deduplicated by PID and sorted by
// CPU descending. Processes that vanish mid-walk are silently skipped.
func SliceProcs(panePIDs []int) ([]ProcInfo, error) {
	visited := make(map[int32]bool)
	var out []ProcInfo

	for _, pid := range panePIDs {
		collectTree(int32(pid), visited, &out)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].CPU > out[j].CPU
	})

	return out, nil
}

// collectTree recursively walks the process tree rooted at pid, appending a
// ProcInfo for each unseen process. Already-visited PIDs are skipped to guard
// against cycles.
func collectTree(pid int32, visited map[int32]bool, out *[]ProcInfo) {
	if visited[pid] {
		return
	}
	visited[pid] = true

	p, err := goproc.NewProcess(pid)
	if err != nil {
		// Process vanished — skip.
		return
	}

	info := snapshot(p)
	*out = append(*out, info)

	kids, _ := p.Children()
	for _, kid := range kids {
		collectTree(kid.Pid, visited, out)
	}
}

// snapshot builds a ProcInfo from a live process handle. Fields that cannot be
// read (e.g. because the process is privileged or briefly gone) are left at
// their zero values.
func snapshot(p *goproc.Process) ProcInfo {
	info := ProcInfo{PID: int(p.Pid)}

	if ppid, err := p.Ppid(); err == nil {
		info.PPID = int(ppid)
	}

	if cpu, err := p.CPUPercent(); err == nil {
		info.CPU = cpu
	}

	if mi, err := p.MemoryInfo(); err == nil && mi != nil {
		info.MemMB = float64(mi.RSS) / (1024 * 1024)
	}

	if cmd, err := p.Cmdline(); err == nil {
		info.Cmd = cmd
	}

	return info
}

// Kill sends SIGTERM to the process with the given PID.
func Kill(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}

// KillSubtree sends SIGKILL to all descendants of pid (deepest first) and
// then to pid itself. Already-dead processes (ESRCH) are tolerated.
func KillSubtree(pid int) error {
	// Collect all descendant PIDs in DFS post-order (deepest first).
	visited := make(map[int32]bool)
	var order []int32
	gatherPostOrder(int32(pid), visited, &order)

	// order contains the full subtree including pid itself, in post-order
	// (leaves first, root last). Kill each in that order.
	for _, p := range order {
		err := syscall.Kill(int(p), syscall.SIGKILL)
		if err != nil && err != syscall.ESRCH {
			// Best-effort: keep going even on errors.
			_ = err
		}
	}

	return nil
}

// gatherPostOrder does a DFS and appends PIDs to 'out' in post-order
// (children appended before their parent), so the result is deepest-first.
// This covers pid itself as the last element.
func gatherPostOrder(pid int32, visited map[int32]bool, out *[]int32) {
	if visited[pid] {
		return
	}
	visited[pid] = true

	p, err := goproc.NewProcess(pid)
	if err != nil {
		// Process already gone — nothing to kill.
		return
	}

	kids, _ := p.Children()
	for _, kid := range kids {
		gatherPostOrder(kid.Pid, visited, out)
	}

	// Append after children so children come first (deepest-first kill order).
	*out = append(*out, pid)
}
