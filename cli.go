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
	case "config":
		return cliConfig(svc, args[1:])
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
  %[3]s config <subcommand>  Inspect or edit the configuration ('config' for details)
  %[3]s rescan               Scan the configured globs and refresh the project cache
  %[3]s list-projects        Print the cached project list
  %[3]s version              Print the version and exit

Environment:
  %-22[4]s State directory (default: ~/.%[3]s)
`,
		appName, version, appName, envHome)
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

func cliConfig(svc *Service, args []string) int {
	if len(args) == 0 {
		printConfigUsage()
		return exitUsage
	}

	sub, rest := args[0], args[1:]
	switch sub {
	case "show":
		cfg, err := svc.ShowConfig()
		if err != nil {
			return fail(err)
		}
		return printJSON(cfg)

	case "schema":
		return printConfigSchema(svc.ConfigSchema())

	case "set", "add", "remove", "unset":
		if len(rest) == 0 {
			fmt.Fprintf(os.Stderr, "error: %s requires a property name\n", sub)
			printConfigUsage()
			return exitUsage
		}
		result, err := svc.EditConfig(ConfigEdit{
			Operation: ConfigOp(sub),
			Property:  rest[0],
			Values:    rest[1:],
		})
		if err != nil {
			return fail(err)
		}
		for _, n := range result.Notes {
			fmt.Fprintln(os.Stderr, "note:", n)
		}
		fmt.Fprintf(os.Stderr, "config updated — run '%s rescan' to apply it\n", appName)
		return printJSON(result.Config)

	default:
		fmt.Fprintf(os.Stderr, "unknown config subcommand: %s\n", sub)
		printConfigUsage()
		return exitUsage
	}
}

func printConfigUsage() {
	fmt.Fprintf(os.Stderr, `Usage:
  %[1]s config show                            Print the stored configuration
  %[1]s config schema                          List the editable properties
  %[1]s config set <property> [<value>...]     Replace a property's value
  %[1]s config add <property> <value>...       Append values
  %[1]s config remove <property> <value>...    Drop values
  %[1]s config unset <property>                Delete a property, restoring its default

'set' with no values writes an explicitly empty list, which is not the same as
'unset': for project_markers, empty turns filtering off while unset restores
the built-in markers.
`, appName)
}

func printConfigSchema(specs []PropertySpec) int {
	for _, s := range specs {
		fmt.Printf("%s  (%s)\n", s.Name, s.Type)
		fmt.Printf("    %s\n", s.Description)
		if s.Note != "" {
			fmt.Printf("    %s\n", s.Note)
		}
	}
	return exitOK
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
