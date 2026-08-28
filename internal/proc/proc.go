// Package proc provides process tree sampling and termination utilities.
// It walks descendant trees of given pane PIDs (as returned by
// internal/tmuxctl.PanePIDs) to report CPU/memory usage and kill runaway
// processes.
package proc

import (
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"syscall"

	goproc "github.com/shirou/gopsutil/v4/process"
)

func TerminalHasForegroundCommand(pid int) (bool, error) {
	output, err := exec.Command("ps", "-o", "pgid=,tpgid=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return false, fmt.Errorf("read terminal process groups for %d: %w", pid, err)
	}
	return parseTerminalProcessGroups(string(output))
}

func TerminalForegroundCommands(pids []int) (map[int]bool, error) {
	busy := make(map[int]bool, len(pids))
	if len(pids) == 0 {
		return busy, nil
	}
	values := make([]string, 0, len(pids))
	for _, pid := range pids {
		values = append(values, strconv.Itoa(pid))
	}
	output, err := exec.Command("ps", "-o", "pid=,pgid=,tpgid=", "-p", strings.Join(values, ",")).Output()
	if err != nil {
		return nil, fmt.Errorf("read terminal process groups: %w", err)
	}
	return parseTerminalProcessGroupsByPID(string(output))
}

func parseTerminalProcessGroupsByPID(output string) (map[int]bool, error) {
	busy := make(map[int]bool)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 {
			return nil, fmt.Errorf("unexpected terminal process group output %q", strings.TrimSpace(line))
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			return nil, fmt.Errorf("parse process ID %q: %w", fields[0], err)
		}
		processGroup, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, fmt.Errorf("parse process group %q: %w", fields[1], err)
		}
		foregroundProcessGroup, err := strconv.Atoi(fields[2])
		if err != nil {
			return nil, fmt.Errorf("parse foreground process group %q: %w", fields[2], err)
		}
		busy[pid] = processGroup != foregroundProcessGroup
	}
	return busy, nil
}

func parseTerminalProcessGroups(output string) (bool, error) {
	fields := strings.Fields(output)
	if len(fields) != 2 {
		return false, fmt.Errorf("unexpected terminal process group output %q", strings.TrimSpace(output))
	}
	processGroup, err := strconv.Atoi(fields[0])
	if err != nil {
		return false, fmt.Errorf("parse process group %q: %w", fields[0], err)
	}
	foregroundProcessGroup, err := strconv.Atoi(fields[1])
	if err != nil {
		return false, fmt.Errorf("parse foreground process group %q: %w", fields[1], err)
	}
	return processGroup != foregroundProcessGroup, nil
}

// ProcInfo holds snapshot data for a single process.
type ProcInfo struct {
	PID   int
	PPID  int
	CPU   float64 // cumulative CPU percent since process start (non-blocking)
	MemMB float64 // RSS in MiB
	Cmd   string  // command line (may be truncated)
	CWD   string
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

func collectTree(pid int32, visited map[int32]bool, out *[]ProcInfo) {
	if visited[pid] {
		return
	}
	visited[pid] = true
	process, err := goproc.NewProcess(pid)
	if err != nil {
		return
	}
	*out = append(*out, snapshot(process))
	children, _ := process.Children()
	for _, child := range children {
		collectTree(child.Pid, visited, out)
	}
}

func SliceProcTrees(rootPIDs []int) (map[int][]ProcInfo, error) {
	trees := make(map[int][]ProcInfo, len(rootPIDs))
	if len(rootPIDs) == 0 {
		return trees, nil
	}
	processes, err := goproc.Processes()
	if err != nil {
		return nil, fmt.Errorf("list processes: %w", err)
	}
	processByPID := make(map[int32]*goproc.Process, len(processes))
	childrenByParent := make(map[int32][]int32)
	for _, process := range processes {
		processByPID[process.Pid] = process
		parentPID, parentErr := process.Ppid()
		if parentErr == nil {
			childrenByParent[parentPID] = append(childrenByParent[parentPID], process.Pid)
		}
	}
	for _, rootPID := range rootPIDs {
		visited := make(map[int32]bool)
		tree := make([]ProcInfo, 0)
		collectIndexedTree(int32(rootPID), processByPID, childrenByParent, visited, &tree)
		sort.Slice(tree, func(i, j int) bool {
			return tree[i].CPU > tree[j].CPU
		})
		trees[rootPID] = tree
	}
	return trees, nil
}

func collectIndexedTree(pid int32, processes map[int32]*goproc.Process, children map[int32][]int32, visited map[int32]bool, out *[]ProcInfo) {
	if visited[pid] {
		return
	}
	visited[pid] = true
	process := processes[pid]
	if process == nil {
		return
	}
	*out = append(*out, snapshot(process))
	for _, childPID := range children[pid] {
		collectIndexedTree(childPID, processes, children, visited, out)
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

	if cwd, err := p.Cwd(); err == nil {
		info.CWD = cwd
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
