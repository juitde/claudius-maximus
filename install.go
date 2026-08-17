package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// mcpCommand is the argument the registered server is invoked with.
//
// The server is a subcommand rather than what running the binary bare does.
// Bare-invocation is the usual convention, but it means a human typing the
// command name gets a process silently waiting on stdin. Being explicit costs
// one word in the registration and makes both failure modes obvious: a human
// gets usage, and a host configured without the word gets an immediate
// non-zero exit rather than usage text mixed into the protocol stream.
const mcpCommand = "mcp"

func cliInstall(svc *Service, args []string) int {
	fs := newFlagSet("install")
	name := fs.String("name", appName, "name to register the MCP server under")
	scope := fs.String("scope", "user", "MCP scope: user, project or local")
	force := fs.Bool("force", false,
		"replace an existing registration under this name and scope, instead of failing")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	// Check first, so a failure leaves no half-made registration pointing at
	// something that cannot work.
	self, err := selfPath()
	if err != nil {
		return fail(err)
	}
	if _, err := exec.LookPath(svc.claudeBin); err != nil {
		return fail(fmt.Errorf("%q not found in PATH — install Claude Code, or set %s",
			svc.claudeBin, envClaudeBin))
	}

	if *force {
		// claude mcp add refuses a duplicate name+scope outright; it does not
		// overwrite. Removing first — and ignoring the error, since "nothing
		// registered yet" is the common case — is what makes --force actually
		// replace a stale registration (for instance after this binary moved)
		// rather than just repeating the same failure.
		_ = exec.Command(svc.claudeBin, "mcp", "remove", *name, "--scope", *scope).Run()
	}

	cmd := exec.Command(svc.claudeBin, "mcp", "add", "--scope", *scope, *name, "--", self, mcpCommand)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fail(fmt.Errorf(
			"claude mcp add failed: %w (if a registration already exists under this name and scope, rerun with --force to replace it)",
			err))
	}

	fmt.Printf("\nRegistered %q (scope %s) pointing at %s\n", *name, *scope, self)
	if _, statErr := os.Stat(svc.configPath); os.IsNotExist(statErr) {
		fmt.Printf("Next: %s config add project_globs '~/dev/*' && %s rescan\n", appName, appName)
	}
	return exitOK
}

func cliUninstall(svc *Service, args []string) int {
	fs := newFlagSet("uninstall")
	name := fs.String("name", appName, "name the MCP server was registered under")
	scope := fs.String("scope", "user", "MCP scope: user, project or local")
	force := fs.Bool("force", false, "remove the registration even while environments are running")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	// Running environments block removal by default. Even without touching
	// state, unregistering means they can no longer be reached through MCP,
	// and that should not happen without being asked for.
	environments, err := svc.ListEnvironments()
	switch {
	case err != nil:
		// A broken registry must not block the removal that might be the way
		// out of it, so this is a warning rather than a stop.
		fmt.Fprintln(os.Stderr, "warning: could not check running environments:", err)
	case len(environments) > 0 && !*force:
		fmt.Fprintf(os.Stderr, "Refusing to remove: %s still running:\n",
			plural(len(environments), "environment"))
		for _, env := range environments {
			fmt.Fprintf(os.Stderr, "  %s  %s\n", env.ProjectName, shortenPath(env.ProjectPath))
		}
		fmt.Fprintf(os.Stderr, "Stop them first, or pass --force to leave them running unmanaged.\n")
		return exitError
	case len(environments) > 0:
		fmt.Fprintf(os.Stderr, "warning: %s left running and no longer reachable through MCP\n",
			plural(len(environments), "environment"))
	}

	cmd := exec.Command(svc.claudeBin, "mcp", "remove", *name, "--scope", *scope)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fail(fmt.Errorf("claude mcp remove failed (it may not have been registered): %w", err))
	}
	fmt.Printf("Removed %q (scope %s)\n", *name, *scope)
	return exitOK
}

// selfPath returns this executable's path with symlinks resolved.
//
// Resolving matters because the registration outlives the shell that made it:
// a path through a symlink that later moves would leave the host launching
// something that is no longer there. It does not survive the target itself
// moving — no path-based registration does.
func selfPath() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("determine own path: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	if within, _ := filepath.Rel(os.TempDir(), self); within != "" && !filepath.IsAbs(within) &&
		!hasParentTraversal(within) {
		return "", fmt.Errorf("this binary is in a temporary directory (%s); "+
			"move it somewhere permanent first", self)
	}
	return self, nil
}

func hasParentTraversal(rel string) bool {
	return len(rel) >= 2 && rel[:2] == ".."
}
