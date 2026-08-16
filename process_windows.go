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
// whose handle is still held elsewhere can read as alive. Good enough here —
// nothing else on the machine has a reason to hold a handle on a claude
// process this tool started — but a real signal-0 equivalent via
// golang.org/x/sys/windows would close the gap if it ever mattered.
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
//
// /T covers the tree, since a spawn may involve more than one process. /F is
// the deliberate asymmetry with Unix, where SIGTERM asks politely: without it
// taskkill posts WM_CLOSE, which a console application with no message loop
// simply never sees, and the stop would silently do nothing. Windows offers no
// graceful signal for an unrelated console process, so the choice is between
// forceful and ineffective.
//
// The cost is that claude does not get to print its shutdown message. Nothing
// is lost by that: the environment it preserves is server-side state keyed by
// the directory, not something the exiting process writes down.
func terminateProcess(pid int) error {
	if pid <= 0 {
		return nil
	}
	return exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
}
