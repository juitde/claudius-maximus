package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// The stub is built once per test binary. Building it per test would add a
// compile to every case that spawns anything, which is most of them.
var (
	stubOnce sync.Once
	stubPath string
	stubErr  error
)

// claudeStub returns the path to a built stand-in for the claude binary.
//
// A compiled program rather than a shell script, so that the spawn, liveness
// and termination paths run on every platform. With a shell stub they were
// skipped on Windows, which meant the platform-specific process handling —
// setDetached, processAlive, terminateProcess — was never executed anywhere.
func claudeStub(t *testing.T) string {
	t.Helper()

	stubOnce.Do(func() {
		dir, err := os.MkdirTemp("", "claudestub")
		if err != nil {
			stubErr = err
			return
		}
		stubPath = filepath.Join(dir, "claude")
		if runtime.GOOS == "windows" {
			stubPath += ".exe"
		}
		out, err := exec.Command("go", "build", "-o", stubPath, "./testdata/claudestub").CombinedOutput()
		if err != nil {
			stubErr = &stubBuildError{err: err, output: string(out)}
		}
	})

	if stubErr != nil {
		t.Fatalf("build claude stub: %v", stubErr)
	}
	return stubPath
}

type stubBuildError struct {
	err    error
	output string
}

func (e *stubBuildError) Error() string { return e.err.Error() + "\n" + e.output }

// stubEnv configures the stub for one test. The code under test builds its own
// command line, so the child is steered through the environment it inherits.
func stubEnv(t *testing.T, values map[string]string) {
	t.Helper()
	// Every variable is set, including to empty, so one test cannot inherit
	// another's configuration through the process environment.
	for _, name := range []string{
		"CLAUDESTUB_BANNER",
		"CLAUDESTUB_STDERR",
		"CLAUDESTUB_EXIT_CODE",
		"CLAUDESTUB_LIFETIME",
		"CLAUDESTUB_VERSION",
		"CLAUDESTUB_MCP_LIST",
	} {
		t.Setenv(name, values[name])
	}
}
