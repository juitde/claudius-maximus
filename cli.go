package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// Exit codes.
const (
	exitOK    = 0
	exitError = 1 // the command ran but failed
	exitUsage = 2 // the command was invoked wrongly
)

// runCLI dispatches a command. Every command here is a thin adapter: parse
// flags, call a Service method, print the result. Behaviour lives in
// service.go so that the MCP tools added later cannot drift from it.
func runCLI(svc *Service, args []string) int {
	if len(args) == 0 {
		printUsage()
		return exitUsage
	}

	switch cmd := args[0]; cmd {
	case "version":
		fmt.Println(appName, version)
		return exitOK
	case "list-projects":
		return cliListProjects(svc)
	case "rescan":
		return cliRescan(svc)
	case "help", "-h", "--help":
		printUsage()
		return exitOK
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		printUsage()
		return exitUsage
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `%s %s

Usage:
  %s rescan           Scan the configured globs and refresh the project cache
  %s list-projects    Print the cached project list
  %s version          Print the version and exit

Environment:
  %-22s State directory (default: ~/.%s)
`,
		appName, version,
		appName, appName, appName,
		envHome, appName)
}

func cliListProjects(svc *Service) int {
	cache, err := svc.ListProjects()
	if err != nil {
		return fail(err)
	}
	// The hint goes to stderr so that stdout stays parseable JSON.
	if len(cache.Projects) == 0 {
		fmt.Fprintf(os.Stderr, "note: project cache is empty — run '%s rescan'\n", appName)
	}
	return printJSON(cache)
}

func cliRescan(svc *Service) int {
	result, err := svc.Rescan()
	if err != nil {
		return fail(err)
	}
	for _, w := range result.Warnings {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	return printJSON(result)
}

// --- helpers ---

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "error:", err)
	return exitError
}

func printJSON(v any) int {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintln(os.Stderr, "error: encoding output:", err)
		return exitError
	}
	return exitOK
}
