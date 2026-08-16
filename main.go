package main

import (
	"os"
)

func main() {
	// Intercepted before any normal dispatch, CLI or MCP: this subcommand is
	// not user-facing, and handleDelayedKill always exits on its own.
	if len(os.Args) > 1 && os.Args[1] == delayedKillArg {
		handleDelayedKill(os.Args[2:])
		return // unreachable
	}
	os.Exit(run(os.Args[1:]))
}

// run is separated from main so tests can exercise dispatch without
// terminating the test binary.
func run(args []string) int {
	stateDir, err := resolveStateDir()
	if err != nil {
		return fail(err)
	}
	if err := ensureLayout(stateDir); err != nil {
		return fail(err)
	}

	return runCLI(newService(stateDir), args)
}
