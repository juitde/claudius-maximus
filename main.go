package main

import (
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
	if err := ensureLayout(stateDir); err != nil {
		return fail(err)
	}

	return runCLI(newService(stateDir), args)
}
