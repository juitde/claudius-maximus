package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestInstallBinary proves the one property the whole self-update design
// depends on: replacing a running executable's file is safe. It swaps a
// live process's own file out from under it and checks two things - the
// already-running instance is undisturbed, and a fresh launch afterward
// runs the new content, not the old.
//
// claudeStub and builtSelf give two different real, already-buildable
// binaries to use as "old" and "new" content, rather than inventing a third
// fixture just for this.
func TestInstallBinary(t *testing.T) {
	stubEnv(t, map[string]string{"CLAUDESTUB_LIFETIME": "1"})

	oldContent, err := os.ReadFile(claudeStub(t))
	if err != nil {
		t.Fatalf("read stub binary: %v", err)
	}
	newContent, err := os.ReadFile(builtSelf(t))
	if err != nil {
		t.Fatalf("read self binary: %v", err)
	}

	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	target := filepath.Join(t.TempDir(), "target"+ext)
	if err := os.WriteFile(target, oldContent, 0o755); err != nil {
		t.Fatalf("write target: %v", err)
	}

	cmd := exec.Command(target)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Start(); err != nil {
		t.Fatalf("start target: %v", err)
	}
	// Not required for correctness (the file is already open for execution
	// the instant Start returns) but gives the process a moment to settle
	// before the swap, so the test isn't racing process creation itself.
	time.Sleep(200 * time.Millisecond)

	if err := installBinary(target, newContent, 0o755); err != nil {
		t.Fatalf("installBinary: %v", err)
	}

	if err := cmd.Wait(); err != nil {
		t.Fatalf("running instance was disrupted by the swap: %v", err)
	}
	if !strings.Contains(stdout.String(), "stub args:") {
		t.Errorf("running instance's own output looks wrong after the swap: %q", stdout.String())
	}

	out, err := exec.Command(target, "version").CombinedOutput()
	if err != nil {
		t.Fatalf("run target after swap: %v\n%s", err, out)
	}
	if !strings.HasPrefix(string(out), appName+" ") {
		t.Errorf("target still runs the old content after installBinary; got %q", out)
	}
}

// TestInstallBinaryStaleTempFile guards the chmod-after-write fix directly:
// a leftover temp file from an interrupted previous run must not leave the
// installed binary silently non-executable.
func TestInstallBinaryStaleTempFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not meaningful on Windows")
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatalf("write target: %v", err)
	}
	// Simulate writeFileAtomic's own leftover temp file from a prior,
	// interrupted run, created with the wrong (non-executable) permissions.
	if err := os.WriteFile(target+".tmp", []byte("stale"), 0o600); err != nil {
		t.Fatalf("write stale tmp file: %v", err)
	}

	if err := installBinary(target, []byte("new"), 0o755); err != nil {
		t.Fatalf("installBinary: %v", err)
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("target is not executable after installBinary: mode %v", info.Mode())
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(data) != "new" {
		t.Errorf("target content = %q, want %q", data, "new")
	}
}
