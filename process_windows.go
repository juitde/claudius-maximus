//go:build windows

package main

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

func setDetached(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

// processAlive uses the fact that os.FindProcess opens a handle on Windows and
// fails when there is nothing to open — unlike on Unix, where it always
// succeeds and a signal is needed to tell.
//
// This is an approximation: a PID belonging to a process that has exited but
// whose handle is still held elsewhere can read as alive. It is good enough
// here, because the plain-process path is the fallback for machines without a
// multiplexer, and where tmux or screen is available their own session lookup
// answers the question properly.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	proc.Release()
	return true
}

// terminateProcess shells out to taskkill, which reaches a process the caller
// did not create — something Windows offers no direct equivalent for.
func terminateProcess(pid int) error {
	if pid <= 0 {
		return nil
	}
	return exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T").Run()
}
