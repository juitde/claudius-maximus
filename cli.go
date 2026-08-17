package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"
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
		return cliListProjects(svc, args[1:])
	case "rescan":
		return cliRescan(svc, args[1:])
	case "start":
		return cliStart(svc, args[1:])
	case "stop":
		return cliStop(svc, args[1:])
	case "list":
		return cliList(svc, args[1:])
	case mcpCommand:
		return cliServeMCP(svc, args[1:])
	case "install":
		return cliInstall(svc, args[1:])
	case "uninstall":
		return cliUninstall(svc, args[1:])
	case "self-update":
		return cliSelfUpdate(svc, args[1:])
	case "config":
		return cliConfig(svc, args[1:])
	case "doctor":
		return cliDoctor(svc, args[1:])
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
  %[3]s start --project <name>   Start a remote-control environment for a project
  %[3]s stop  --project <name>   Stop it again
  %[3]s list                     Show the environments now running

  %[3]s install              Register as an MCP server with Claude Code
  %[3]s uninstall            Remove that registration
  %[3]s mcp                  Run the MCP server over stdio (what install registers)
  %[3]s self-update          Replace this binary with the latest (or --version) release

  %[3]s doctor [--json]      Report on the setup and preview what a rescan would do
  %[3]s config <subcommand>  Inspect or edit the configuration ('config' for details)
  %[3]s rescan               Scan the configured globs and refresh the project cache
  %[3]s list-projects        Print the cached project list
  %[3]s version              Print the version and exit

Environment:
  %-22[4]s State directory (default: ~/.%[3]s)
`,
		appName, version, appName, envHome)
}

func cliListProjects(svc *Service, args []string) int {
	mode, ok := parseOutputMode("list-projects", args)
	if !ok {
		return exitUsage
	}

	cache, err := svc.ListProjects()
	if err != nil {
		return fail(err)
	}
	if mode == outputJSON {
		return printJSON(cache)
	}

	if len(cache.Projects) == 0 {
		fmt.Printf("No projects cached. Run '%s rescan'.\n", appName)
		return exitOK
	}
	fmt.Printf("%s, scanned %s\n\n", plural(len(cache.Projects), "project"), humanizeSince(cache.ScannedAt))
	printProjectList(cache.Projects, "  ")
	return exitOK
}

func cliRescan(svc *Service, args []string) int {
	mode, ok := parseOutputMode("rescan", args)
	if !ok {
		return exitUsage
	}

	result, err := svc.Rescan()
	if err != nil {
		return fail(err)
	}
	if mode == outputJSON {
		return printJSON(result)
	}

	fmt.Println(describeChanges(len(result.Added), len(result.Removed), len(result.Renamed), result.Unchanged))

	// The default is exactly that one line. Which projects changed is a real
	// question, but it is the second one, and answering it unprompted turns a
	// routine rescan back into a wall of output — on a first run every project
	// is an addition.
	if mode == outputVerbose {
		printProjectGroup("added", result.Added)
		printProjectGroup("removed", result.Removed)
		printRenames(result.Renamed)
		printEnvironmentRenames(result.RenamedEnvironments)
		printRejections(summarizeRejections(result.Rejected))
		printPruned(result.Pruned)
	}

	for _, w := range result.Warnings {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	return exitOK
}

// targetFlags registers the project selectors shared by start and stop.
func targetFlags(fs *flag.FlagSet) (name, path *string) {
	name = fs.String("project", "", "project name from list-projects")
	path = fs.String("path", "", "project directory, as an alternative to --project")
	return name, path
}

func cliStart(svc *Service, args []string) int {
	fs := newFlagSet("start")
	name, path := targetFlags(fs)
	spawn := fs.String("spawn", "", "override the configured spawn mode for this start")
	asJSON := fs.Bool("json", false, "print the result as JSON")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	result, err := svc.StartEnvironment(StartArgs{
		Target:    ProjectTarget{Name: *name, Path: *path},
		SpawnMode: SpawnMode(*spawn),
	})
	if err != nil {
		return fail(err)
	}
	if *asJSON {
		return printJSON(result)
	}

	verb := "Started"
	if result.AlreadyRunning {
		verb = "Already running:"
	}
	fmt.Printf("%s %s\n  %s\n", verb, result.Environment.ProjectName, result.Environment.URL)
	return exitOK
}

func cliStop(svc *Service, args []string) int {
	fs := newFlagSet("stop")
	name, path := targetFlags(fs)
	asJSON := fs.Bool("json", false, "print the result as JSON")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	result, err := svc.StopEnvironment(ProjectTarget{Name: *name, Path: *path})
	if err != nil {
		return fail(err)
	}
	if *asJSON {
		return printJSON(result)
	}
	fmt.Printf("Stopped %s\n", result.Environment.ProjectName)
	return exitOK
}

func cliList(svc *Service, args []string) int {
	mode, ok := parseOutputMode("list", args)
	if !ok {
		return exitUsage
	}

	environments, err := svc.ListEnvironments()
	if err != nil {
		return fail(err)
	}
	if mode == outputJSON {
		return printJSON(environments)
	}
	if len(environments) == 0 {
		fmt.Printf("No environments running. Start one with '%s start --project <name>'.\n", appName)
		return exitOK
	}

	fmt.Printf("%s running\n\n", plural(len(environments), "environment"))
	printEnvironments(environments, mode)
	return exitOK
}

// cliServeMCP runs the protocol server. Nothing on this path may write to
// stdout: that is the transport, and a stray line of human output corrupts it.
func cliServeMCP(svc *Service, args []string) int {
	if len(args) > 0 {
		fmt.Fprintf(os.Stderr, "error: %s takes no arguments\n", mcpCommand)
		return exitUsage
	}
	if err := serveMCP(context.Background(), svc); err != nil {
		return fail(err)
	}
	return exitOK
}

func cliDoctor(svc *Service, args []string) int {
	fs := newFlagSet("doctor")
	asJSON := fs.Bool("json", false, "print the report as JSON instead of text")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	report := svc.Doctor()
	if *asJSON {
		if code := printJSON(report); code != exitOK {
			return code
		}
	} else {
		printDoctorHuman(report)
	}

	// Only a hard failure is worth a non-zero exit. Warnings describe things
	// the user may well have chosen on purpose.
	if report.failed() {
		return exitError
	}
	return exitOK
}

func cliConfig(svc *Service, args []string) int {
	if len(args) == 0 {
		printConfigUsage()
		return exitUsage
	}

	sub, rest := args[0], args[1:]
	switch sub {
	case "show":
		mode, ok := parseOutputMode("config show", rest)
		if !ok {
			return exitUsage
		}
		cfg, err := svc.ShowConfig()
		if err != nil {
			return fail(err)
		}
		if mode == outputJSON {
			return printJSON(cfg)
		}
		printConfigHuman(cfg, mode)
		return exitOK

	case "schema":
		return printConfigSchema(svc.ConfigSchema())

	case "set", "add", "remove", "unset":
		// Flags are stripped before the property and values, so that
		// "config add project_globs '~/dev/*' --json" works either way round.
		mode, operands, ok := splitOutputFlags(sub, rest)
		if !ok {
			return exitUsage
		}
		if len(operands) == 0 {
			fmt.Fprintf(os.Stderr, "error: %s requires a property name\n", sub)
			printConfigUsage()
			return exitUsage
		}
		result, err := svc.EditConfig(ConfigEdit{
			Operation: ConfigOp(sub),
			Property:  operands[0],
			Values:    operands[1:],
		})
		if err != nil {
			return fail(err)
		}
		for _, n := range result.Notes {
			fmt.Fprintln(os.Stderr, "note:", n)
		}
		if mode == outputJSON {
			return printJSON(result.Config)
		}
		printConfigHuman(&result.Config, mode)
		fmt.Printf("\nRun '%s rescan' to apply.\n", appName)
		return exitOK

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

// printConfigHuman shows each property's effective value, marking which come
// from the built-in defaults — information the stored file cannot give, since
// an unset property is simply absent from it.
func printConfigHuman(cfg *Config, mode outputMode) {
	specs := configSchema()
	width := columnWidth(specs, func(s PropertySpec) string { return s.Name })

	for _, spec := range specs {
		values, isDefault := effectiveProperty(cfg, spec.Name)

		origin := ""
		if isDefault {
			origin = "(default) "
		}
		rendered := describeList(values)
		if mode == outputVerbose {
			rendered = strings.Join(values, ", ")
			if len(values) == 0 {
				rendered = "(empty)"
			}
		}
		fmt.Printf("%-*s  %s%s\n", width, spec.Name, origin, rendered)
	}
}

// effectiveProperty returns the values in force for a property and whether
// they come from the defaults rather than the configuration.
func effectiveProperty(cfg *Config, property string) (values []string, isDefault bool) {
	field, err := configField(cfg, property)
	if err != nil {
		return nil, false
	}
	switch field.Kind() {
	case reflect.Slice:
		// nil means unset; an empty list is a decision and stands.
		if stored, _ := field.Interface().([]string); stored != nil {
			return stored, false
		}
	case reflect.String:
		if stored := field.String(); stored != "" {
			return []string{stored}, false
		}
	}
	if meta := propertyMeta[property]; meta.defaults != nil {
		return meta.defaults(), true
	}
	return nil, false
}

func printConfigSchema(specs []PropertySpec) int {
	for _, s := range specs {
		fmt.Printf("%s  (%s)\n", s.Name, s.Type)
		fmt.Printf("    %s\n", s.Description)
		if len(s.Allowed) > 0 {
			fmt.Printf("    one of: %s\n", strings.Join(s.Allowed, ", "))
		}
		if s.Note != "" {
			fmt.Printf("    %s\n", s.Note)
		}
	}
	return exitOK
}

// --- helpers ---

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

// parseOutputMode handles the --json / --verbose pair that every reporting
// command accepts, so the three modes mean the same thing everywhere.
func parseOutputMode(command string, args []string) (outputMode, bool) {
	fs := newFlagSet(command)
	asJSON := fs.Bool("json", false, "print the full result as JSON")
	verbose := fs.Bool("verbose", false, "print every detail rather than a summary")
	if err := fs.Parse(args); err != nil {
		return outputSummary, false
	}
	if *asJSON && *verbose {
		fmt.Fprintln(os.Stderr, "error: --json and --verbose cannot be combined; --json is always complete")
		return outputSummary, false
	}

	switch {
	case *asJSON:
		return outputJSON, true
	case *verbose:
		return outputVerbose, true
	default:
		return outputSummary, true
	}
}

// splitOutputFlags separates the output flags from positional operands, so a
// command taking arbitrary values can still accept --json anywhere. Go's flag
// package stops at the first non-flag argument, which would otherwise make
// flag position significant for no reason the user could guess.
func splitOutputFlags(command string, args []string) (outputMode, []string, bool) {
	var flags, operands []string
	for _, a := range args {
		if a == "--json" || a == "-json" || a == "--verbose" || a == "-verbose" {
			flags = append(flags, a)
			continue
		}
		operands = append(operands, a)
	}
	mode, ok := parseOutputMode(command, flags)
	return mode, operands, ok
}

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
