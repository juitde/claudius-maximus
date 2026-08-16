package main

import (
	"fmt"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run is separated from main so tests can exercise dispatch without
// terminating the test binary.
func run(args []string) int {
	stateDir, err := resolveStateDir()
	if err != nil {
		return fail(err)
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return fail(fmt.Errorf("create state directory %s: %w", stateDir, err))
	}

	return runCLI(newService(stateDir), args)
}
