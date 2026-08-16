// Command claudestub stands in for the claude binary in tests.
//
// It is a Go program rather than a shell script so the spawn, liveness and
// termination paths can be exercised on every platform this tool builds for.
// A shell stub meant skipping all of them on Windows, which left the
// platform-specific process handling — the code most likely to be wrong —
// never executed at all.
//
// Behaviour is driven by environment variables, because the code under test
// builds its own command line and the child inherits the test process's
// environment.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	envBanner   = "CLAUDESTUB_BANNER"    // written to stdout; defaults to a connected banner
	envStderr   = "CLAUDESTUB_STDERR"    // written to stderr before exiting
	envExitCode = "CLAUDESTUB_EXIT_CODE" // exit immediately with this code
	envLifetime = "CLAUDESTUB_LIFETIME"  // seconds to stay alive; defaults to 60
	envVersion  = "CLAUDESTUB_VERSION"   // answer for --version
	envMCPList  = "CLAUDESTUB_MCP_LIST"  // answer for `mcp list`
)

const defaultBanner = "Continue coding in the Claude mobile app or " +
	"https://claude.ai/code?environment=env_stub\n"

func main() {
	// The queries doctor makes are answered and nothing else happens, matching
	// how the real command behaves for these.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version":
			fmt.Println(env(envVersion, "2.1.233 (Claude Code)"))
			return
		case "mcp":
			fmt.Println(os.Getenv(envMCPList))
			return
		}
	}

	// Echoed so tests can assert on what the code under test actually passed,
	// rather than on what it was meant to pass.
	cwd, _ := os.Getwd()
	// Reported with symlinks resolved so a caller can compare it against a
	// path it resolved the same way. macOS hands out temp directories under
	// /var, which is a link to /private/var, and the two spellings would
	// otherwise never match.
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = resolved
	}
	fmt.Println("stub cwd:", cwd)
	fmt.Println("stub args:", strings.Join(os.Args[1:], " "))

	if message := os.Getenv(envStderr); message != "" {
		fmt.Fprintln(os.Stderr, message)
	}
	if code := os.Getenv(envExitCode); code != "" {
		parsed, err := strconv.Atoi(code)
		if err != nil {
			parsed = 1
		}
		os.Exit(parsed)
	}

	fmt.Print(env(envBanner, defaultBanner))

	// Stay alive like the real command, which keeps running until stopped.
	seconds, err := strconv.Atoi(env(envLifetime, "60"))
	if err != nil {
		seconds = 60
	}
	time.Sleep(time.Duration(seconds) * time.Second)
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
