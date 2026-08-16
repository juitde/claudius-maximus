//go:build !windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// setDetached puts the child in its own process group, so it survives signals
// aimed at ours and can later be killed as a group.
func setDetached(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// processAlive reports whether a PID is still running. FindProcess always
// succeeds on Unix, so the answer comes from signal 0, which checks for the
// process without disturbing it.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

// terminateProcess signals the whole process group rather than a single PID.
// claude is free to spawn its own children, and setDetached placed it in a
// fresh group precisely so a single signal reaches all of them.
func terminateProcess(pid int) error {
	if pid <= 0 {
		return nil
	}
	return syscall.Kill(-pid, syscall.SIGTERM)
}
