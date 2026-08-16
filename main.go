package main

import (
	"fmt"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run is separated from main so that tests can exercise the dispatch without
// terminating the test binary. Exit codes: 0 = success, 1 = operational
// failure, 2 = usage error.
func run(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 2
	}

	switch args[0] {
	case "version":
		fmt.Println(appName, version)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		printUsage()
		return 2
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `%s %s

Usage:
  %s version    Print the version and exit
`, appName, version, appName)
}
