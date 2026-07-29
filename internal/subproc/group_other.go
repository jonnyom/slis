//go:build !unix

package subproc

import "os/exec"

// setProcessGroup is a no-op where process groups are unavailable.
func setProcessGroup(*exec.Cmd) {}

// killGroup degrades to killing just the child process.
func killGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
