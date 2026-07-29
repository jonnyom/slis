//go:build unix

package subproc

import (
	"os/exec"
	"syscall"
)

// setProcessGroup makes the child the leader of a new process group, so its own
// children join that group and a single signal reaches all of them.
func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// killGroup SIGKILLs the child's whole process group. It falls back to the lone
// process when the group is already gone or was never created.
func killGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	pid := cmd.Process.Pid
	if err := syscall.Kill(-pid, syscall.SIGKILL); err == nil {
		return nil
	}
	return cmd.Process.Kill()
}
