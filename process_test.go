package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// realOutput reproduces what claude 2.1.233 actually prints, including the
// cursor-movement escapes it emits even when redirected to a file, the
// repeated frames that produces, and the documentation link that sits in the
// same output as the one that matters.
const realOutput = "\n·|· Connecting · dummy · HEAD\n" +
	"\x1b[1A\x1b[J·⎯· Connected · dummy · HEAD\n" +
	"    Capacity: 0/32 · New sessions will be created in the current directory\n\n" +
	"Continue coding in the Claude mobile app or https://claude.ai/code?environment=env_01LB1RYiukoQKoCU4JLEVnWA\n" +
	"space to show QR code · w to toggle spawn mode\n" +
	"\x1b[6A\x1b[J·⎯· Connected · dummy · HEAD\n" +
	"    Capacity: 0/32 · New sessions will be created in the current directory\n\n" +
	"Continue coding in the Claude mobile app or https://claude.ai/code?environment=env_01LB1RYiukoQKoCU4JLEVnWA\n" +
	"space to show QR code · w to toggle spawn mode\n"

// spawnPromptOutput is what appears when --spawn is omitted and claude has a
// terminal: it stops and waits, and the only URL on screen is the docs link.
const spawnPromptOutput = "Remote Control is launching in spawn mode, which lets you start new sessions " +
	"in this project from claude.ai/code or the Claude mobile app. " +
	"Learn more: https://code.claude.com/docs/en/remote-control\n\n" +
	"Spawn mode for this project:\n" +
	"  [1] same-dir — sessions share the current directory (default)\n" +
	"  [2] worktree — each session gets an isolated git worktree\n\n" +
	"Choose [1/2] (default: 1): "

func TestURLPatternMatchesRealOutput(t *testing.T) {
	pattern, err := resolveURLPattern()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	logPath := filepath.Join(t.TempDir(), "rc.log")
	mustWrite(t, logPath, realOutput)

	url, environmentID, found := matchLog(logPath, pattern)
	if !found {
		t.Fatal("the pattern did not match claude's actual output")
	}
	if want := "https://claude.ai/code?environment=env_01LB1RYiukoQKoCU4JLEVnWA"; url != want {
		t.Errorf("url = %q, want %q", url, want)
	}
	if want := "env_01LB1RYiukoQKoCU4JLEVnWA"; environmentID != want {
		t.Errorf("environment id = %q, want %q", environmentID, want)
	}
}

func TestURLPatternIgnoresTheDocumentationLink(t *testing.T) {
	// The trap: this output contains a URL, but not the one that means
	// "connected". Matching it would report success while claude sits waiting
	// for a keypress.
	logPath := filepath.Join(t.TempDir(), "rc.log")
	mustWrite(t, logPath, spawnPromptOutput)

	pattern, _ := resolveURLPattern()
	if _, _, found := matchLog(logPath, pattern); found {
		t.Error("the documentation link must not be mistaken for an environment URL")
	}
}

func TestURLPatternRejectsThePathShapedForm(t *testing.T) {
	// The shape the original design assumed. Keeping this as a test records
	// that it was checked against the real thing and is wrong.
	logPath := filepath.Join(t.TempDir(), "rc.log")
	mustWrite(t, logPath, "see https://claude.ai/code/env_01LB1RYiukoQKoCU4JLEVnWA\n")

	pattern, _ := resolveURLPattern()
	if _, _, found := matchLog(logPath, pattern); found {
		t.Error("a path-shaped URL is not the format claude emits")
	}
}

func TestResolveURLPatternOverride(t *testing.T) {
	t.Run("valid override is used", func(t *testing.T) {
		t.Setenv(envURLPattern, `ENV=(custom_[0-9]+)`)

		pattern, err := resolveURLPattern()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		logPath := filepath.Join(t.TempDir(), "rc.log")
		mustWrite(t, logPath, "ENV=custom_42\n")

		url, id, found := matchLog(logPath, pattern)
		if !found || url != "ENV=custom_42" || id != "custom_42" {
			t.Errorf("got %q / %q / %v", url, id, found)
		}
	})

	t.Run("malformed override is reported", func(t *testing.T) {
		t.Setenv(envURLPattern, `(unterminated`)
		if _, err := resolveURLPattern(); err == nil {
			t.Error("expected an error for an invalid expression")
		}
	})

	t.Run("override without a capture group is rejected", func(t *testing.T) {
		// Without one there is nothing to record as the environment ID, and a
		// silently empty ID would be worse than refusing.
		t.Setenv(envURLPattern, `https://claude\.ai/code`)
		if _, err := resolveURLPattern(); err == nil {
			t.Error("expected an error for a pattern with no capture group")
		}
	})
}

func TestClaudeArgsAlwaysPassesSpawnMode(t *testing.T) {
	// Omitting it makes claude prompt as soon as it has a terminal, which
	// would hang if this ever runs inside one with nobody there to answer.
	for _, mode := range []SpawnMode{SpawnSameDir, SpawnWorktree} {
		args := claudeArgs(mode)
		if args[0] != "remote-control" {
			t.Errorf("args = %v, want remote-control first", args)
		}
		if !contains(args, "--spawn="+string(mode)) {
			t.Errorf("args = %v, want --spawn=%s", args, mode)
		}
	}
}

func TestStripANSI(t *testing.T) {
	got := stripANSI(realOutput)
	if strings.Contains(got, "\x1b") {
		t.Error("escape characters survived")
	}
	if !strings.Contains(got, "Connected · dummy · HEAD") {
		t.Error("stripping removed real text")
	}
}

func TestLogTail(t *testing.T) {
	t.Run("shows readable text without escapes", func(t *testing.T) {
		logPath := filepath.Join(t.TempDir(), "rc.log")
		mustWrite(t, logPath, realOutput)

		tail := logTail(logPath)
		if strings.Contains(tail, "\x1b") {
			t.Error("the tail must not carry terminal escapes into an error message")
		}
		if !strings.Contains(tail, "Capacity: 0/32") {
			t.Errorf("tail lost the useful content:\n%s", tail)
		}
	})

	t.Run("is bounded", func(t *testing.T) {
		logPath := filepath.Join(t.TempDir(), "rc.log")
		mustWrite(t, logPath, strings.Repeat("a line of output\n", 500))

		if lines := strings.Count(logTail(logPath), "\n") + 1; lines > logTailLines {
			t.Errorf("tail has %d lines, want at most %d", lines, logTailLines)
		}
	})

	t.Run("missing log does not panic", func(t *testing.T) {
		if got := logTail(filepath.Join(t.TempDir(), "absent.log")); got == "" {
			t.Error("expected a placeholder rather than an empty string")
		}
	})
}

func TestAwaitURL(t *testing.T) {
	pattern, _ := resolveURLPattern()

	t.Run("returns as soon as the URL appears", func(t *testing.T) {
		logPath := filepath.Join(t.TempDir(), "rc.log")
		mustWrite(t, logPath, "")

		go func() {
			time.Sleep(150 * time.Millisecond)
			os.WriteFile(logPath, []byte(realOutput), 0o600)
		}()

		url, id, err := awaitURL(logPath, pattern, 5*time.Second, func() bool { return true })
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != "env_01LB1RYiukoQKoCU4JLEVnWA" || url == "" {
			t.Errorf("got %q / %q", url, id)
		}
	})

	t.Run("reports an early exit instead of waiting out the timeout", func(t *testing.T) {
		logPath := filepath.Join(t.TempDir(), "rc.log")
		mustWrite(t, logPath, "Invalid API key · Please run /login\n")

		started := time.Now()
		_, _, err := awaitURL(logPath, pattern, 30*time.Second, func() bool { return false })
		if err == nil {
			t.Fatal("expected an error")
		}
		if elapsed := time.Since(started); elapsed > 2*time.Second {
			t.Errorf("waited %s for a process that was already gone", elapsed)
		}
		// The error has to carry what claude said, or it explains nothing.
		if !strings.Contains(err.Error(), "Invalid API key") {
			t.Errorf("error does not include the output: %v", err)
		}
	})

	t.Run("timeout mentions the override and the last output", func(t *testing.T) {
		logPath := filepath.Join(t.TempDir(), "rc.log")
		mustWrite(t, logPath, spawnPromptOutput)

		_, _, err := awaitURL(logPath, pattern, 300*time.Millisecond, func() bool { return true })
		if err == nil {
			t.Fatal("expected a timeout")
		}
		for _, want := range []string{envURLPattern, "Choose [1/2]"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error should mention %q: %v", want, err)
			}
		}
	})
}

// --- spawning a real process ---

func TestSpawnPlain(t *testing.T) {
	t.Run("captures the URL and keeps running", func(t *testing.T) {
		stubEnv(t, map[string]string{"CLAUDESTUB_BANNER": realOutput})
		bin := claudeStub(t)

		dir := t.TempDir()
		logPath := filepath.Join(t.TempDir(), "rc.log")

		outcome, err := spawnPlain(spawnSpec{
			ProjectPath: dir,
			ClaudeBin:   bin,
			LogPath:     logPath,
			SpawnMode:   SpawnSameDir,
			Timeout:     10 * time.Second,
		})
		if err != nil {
			t.Fatalf("spawn: %v", err)
		}
		defer terminateProcess(outcome.PID)

		if outcome.EnvironmentID != "env_01LB1RYiukoQKoCU4JLEVnWA" {
			t.Errorf("environment id = %q", outcome.EnvironmentID)
		}
		if outcome.PID <= 0 || !processAlive(outcome.PID) {
			t.Errorf("expected a running process, got pid %d", outcome.PID)
		}
	})

	t.Run("passes the spawn mode through", func(t *testing.T) {
		// The stub echoes its arguments, so the assertion is on what claude
		// would actually have received.
		stubEnv(t, nil)
		bin := claudeStub(t)
		logPath := filepath.Join(t.TempDir(), "rc.log")

		outcome, err := spawnPlain(spawnSpec{
			ProjectPath: t.TempDir(),
			ClaudeBin:   bin,
			LogPath:     logPath,
			SpawnMode:   SpawnWorktree,
			Timeout:     10 * time.Second,
		})
		if err != nil {
			t.Fatalf("spawn: %v", err)
		}
		defer terminateProcess(outcome.PID)

		data, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("read log: %v", err)
		}
		if !strings.Contains(string(data), "--spawn=worktree") {
			t.Errorf("claude was not given the spawn mode:\n%s", data)
		}
	})

	t.Run("runs in the project directory", func(t *testing.T) {
		stubEnv(t, nil)
		bin := claudeStub(t)
		dir := t.TempDir()
		logPath := filepath.Join(t.TempDir(), "rc.log")

		outcome, err := spawnPlain(spawnSpec{
			ProjectPath: dir,
			ClaudeBin:   bin,
			LogPath:     logPath,
			SpawnMode:   SpawnSameDir,
			Timeout:     10 * time.Second,
		})
		if err != nil {
			t.Fatalf("spawn: %v", err)
		}
		defer terminateProcess(outcome.PID)

		data, _ := os.ReadFile(logPath)
		resolved, _ := filepath.EvalSymlinks(dir)
		if !strings.Contains(string(data), resolved) {
			t.Errorf("expected the process to run in %s:\n%s", resolved, data)
		}
	})

	t.Run("a claude that exits is reported with its output", func(t *testing.T) {
		stubEnv(t, map[string]string{
			"CLAUDESTUB_STDERR":    "Credit balance too low",
			"CLAUDESTUB_EXIT_CODE": "1",
		})
		bin := claudeStub(t)

		_, err := spawnPlain(spawnSpec{
			ProjectPath: t.TempDir(),
			ClaudeBin:   bin,
			LogPath:     filepath.Join(t.TempDir(), "rc.log"),
			SpawnMode:   SpawnSameDir,
			Timeout:     10 * time.Second,
		})
		if err == nil {
			t.Fatal("expected an error")
		}
		if !strings.Contains(err.Error(), "Credit balance too low") {
			t.Errorf("error should carry claude's output: %v", err)
		}
	})

	t.Run("a missing binary fails immediately", func(t *testing.T) {
		_, err := spawnPlain(spawnSpec{
			ProjectPath: t.TempDir(),
			ClaudeBin:   filepath.Join(t.TempDir(), "does-not-exist"),
			LogPath:     filepath.Join(t.TempDir(), "rc.log"),
			SpawnMode:   SpawnSameDir,
			Timeout:     10 * time.Second,
		})
		if err == nil {
			t.Fatal("expected an error")
		}
	})
}

func TestProcessAlive(t *testing.T) {
	if processAlive(0) || processAlive(-1) {
		t.Error("a non-positive pid is never alive")
	}
	if !processAlive(os.Getpid()) {
		t.Error("this test's own process should count as alive")
	}
}
