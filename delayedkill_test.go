package main

import (
	"os/exec"
	"testing"
	"time"
)

// startVictim starts a long-lived stand-in process to be terminated by the
// tests below, cleaned up at the end regardless of what the test under it
// actually does to it.
func startVictim(t *testing.T) int {
	t.Helper()
	stubEnv(t, map[string]string{"CLAUDESTUB_LIFETIME": "30"})

	cmd := exec.Command(claudeStub(t))
	setDetached(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start victim: %v", err)
	}
	pid := cmd.Process.Pid
	go func() { _ = cmd.Wait() }()
	t.Cleanup(func() { _ = terminateProcess(pid) })
	return pid
}

func TestDelayedKill(t *testing.T) {
	pid := startVictim(t)
	if !processAlive(pid) {
		t.Fatal("victim should be alive before the call")
	}

	delayedKill(pid, 20*time.Millisecond)

	if !waitForExit(pid) {
		t.Error("victim should be dead once delayedKill returns")
	}
}

// TestSpawnDelayedKillHasAGracePeriod is the point of the whole mechanism: the
// kill must not happen before the caller has had a chance to respond. Exercised
// through the real built binary, so this also proves main.go's subcommand
// interception and handleDelayedKill's argument parsing work, not only the
// pure delayedKill logic.
func TestSpawnDelayedKillHasAGracePeriod(t *testing.T) {
	pid := startVictim(t)
	self := builtSelf(t)

	const delay = 400 * time.Millisecond
	if err := spawnDelayedKill(self, pid, delay); err != nil {
		t.Fatalf("spawnDelayedKill: %v", err)
	}

	// Immediately after spawning the helper the grace period must still be in
	// effect. This is what an in-line kill cannot offer.
	if !processAlive(pid) {
		t.Fatal("victim died before the delay elapsed — there is no grace period")
	}

	time.Sleep(delay + 300*time.Millisecond) // clear margin past the delay
	if processAlive(pid) {
		t.Error("victim is still alive well after the delay should have expired")
	}
}

func TestHandleDelayedKillRejectsBadArguments(t *testing.T) {
	self := builtSelf(t)

	tests := []struct {
		name string
		args []string
	}{
		{"no arguments", nil},
		{"only one argument", []string{"123"}},
		{"pid is not a number", []string{"notanumber", "10ms"}},
		{"delay is not a duration", []string{"123", "notaduration"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{delayedKillArg}, tt.args...)
			err := exec.Command(self, args...).Run()

			exitErr, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatalf("expected the process to exit with an error, got %v", err)
			}
			if exitErr.ExitCode() != 2 {
				t.Errorf("exit code = %d, want 2", exitErr.ExitCode())
			}
		})
	}
}
