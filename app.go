package main

// Identity constants for the whole application. Everything user-visible that
// carries the tool's name derives from these, so a rename stays a one-line
// change instead of a grep-and-hope exercise.
const (
	// appName is the primary binary name and the name this tool registers
	// itself under as an MCP server.
	appName = "claudius-maximus"

	// shortName is the convenience alias installed alongside appName. It is
	// also the prefix for tmux/screen session names, where a short prefix
	// keeps `tmux ls` output readable.
	shortName = "cmax"
)

// version is the build version. Deliberately a var, not a const: linker
// overrides via -ldflags "-X main.version=..." only apply to variables, and a
// const here would silently ignore the release build's version stamp.
var version = "0.1.0-dev"

// envPrefix is prepended to every environment variable this tool reads.
// Deliberately not "CLAUDE_" — that namespace belongs to Claude Code itself
// and collisions there would be silent and confusing.
const envPrefix = "CLAUDIUS_"
