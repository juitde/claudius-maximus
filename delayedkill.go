package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"
)

// delayedKillArg is the hidden subcommand main.go intercepts before normal
// dispatch. It exists so the actual termination happens in a process separate
// from whichever call asked for it.
//
// That separation matters for one specific case: a session running inside the
// environment it is asking to stop calls stop_environment through an MCP
// server process that was itself spawned as a child of that environment, and
// therefore inherits its process group. Killing the group in-line would
// signal that MCP server process too, with no guarantee the JSON-RPC response
// reaches the client before it dies. A self-identification scheme was the
// original design's answer to a related question — resolving session_id when
// omitted — but that question does not arise here: stop_environment always
// takes an explicit project, even when a session is stopping the one it lives
// inside, since it already knows its own directory. Only the delayed part of
// the original design is needed.
const delayedKillArg = "__delayed-kill"

// defaultKillDelay is long enough for an MCP response to be written and read
// before the target dies, and short enough that stopping something still
// feels immediate.
//
// Fixed rather than configurable: nothing so far has needed a different
// value, and an environment variable for a knob nobody turns is exactly the
// kind of complexity this tool tries to avoid.
const defaultKillDelay = 300 * time.Millisecond

// spawnDelayedKill starts a detached helper that waits, then terminates pid.
func spawnDelayedKill(selfBinary string, pid int, delay time.Duration) error {
	cmd := exec.Command(selfBinary, delayedKillArg, strconv.Itoa(pid), delay.String())
	setDetached(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start delayed-kill helper: %w", err)
	}
	// Reaped in the background rather than released, for the same reason as
	// spawnPlain's child: an unreaped process that exits becomes a zombie, and
	// this helper is short-lived by design.
	go func() { _ = cmd.Wait() }()
	return nil
}

// handleDelayedKill is the hidden subcommand's body, run inside the process
// spawnDelayedKill starts. Always exits; never returns.
func handleDelayedKill(args []string) {
	if len(args) != 2 {
		os.Exit(2)
	}
	pid, err := strconv.Atoi(args[0])
	if err != nil {
		os.Exit(2)
	}
	delay, err := time.ParseDuration(args[1])
	if err != nil {
		os.Exit(2)
	}
	delayedKill(pid, delay)
	os.Exit(0)
}

// delayedKill is the logic on its own, apart from handleDelayedKill's process
// exit, so it can be tested directly without spawning anything.
func delayedKill(pid int, delay time.Duration) {
	time.Sleep(delay)
	_ = terminateProcess(pid)
}
